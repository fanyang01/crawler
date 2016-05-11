package sqlstore

import (
	"errors"
	"net/url"
	"time"

	"github.com/fanyang01/crawler"
	"github.com/fanyang01/crawler/bloom"
	"github.com/jmoiron/sqlx"

	_ "github.com/lib/pq"
)

type SQLStore struct {
	DB     *sqlx.DB
	filter *bloom.Filter
}

type wrapper struct {
	Scheme   string
	Host     string
	Path     string
	Query    string
	Depth    int
	Done     bool
	Status   int
	Last     time.Time
	NumVisit int `db:"num_visit"`
	NumError int `db:"num_error"`
}

func (w *wrapper) ToURL() *crawler.URL {
	u := &crawler.URL{
		URL: url.URL{
			Scheme:   w.Scheme,
			Host:     w.Host,
			RawPath:  w.Path,
			RawQuery: w.Query,
		},
		Depth:    w.Depth,
		Done:     w.Done,
		Status:   w.Status,
		Last:     w.Last,
		NumVisit: w.NumVisit,
		NumRetry: w.NumError,
	}
	return u
}

func (w *wrapper) fromURL(u *crawler.URL) {
	w.Scheme = u.URL.Scheme
	w.Host = u.URL.Host
	w.Path = u.URL.EscapedPath()
	w.Query = u.Query().Encode()
	w.Depth = u.Depth
	w.Done = u.Done
	w.Status = u.Status
	w.Last = u.Last
	w.NumVisit = u.NumVisit
	w.NumError = u.NumRetry
}

const (
	URLSchema = `
CREATE TABLE IF NOT EXISTS url (
	scheme    VARCHAR(16),
	host      VARCHAR(253),
	path      TEXT,
	query     TEXT,
	depth     INT NOT NULL,
	done      BOOLEAN NOT NULL,
	status    INT NOT NULL,
	last      TIMESTAMP NOT NULL,
	num_visit INT NOT NULL,
	num_error INT NOT NULL,
	PRIMARY KEY (scheme, host, path, query)
)`
	CountSchema = `
CREATE TABLE IF NOT EXISTS count (
	url_count    INT NOT NULL,
	finish_count INT NOT NULL,
	error_count  INT NOT NULL,
	visit_count  INT NOT NULL
)
`
)

func New(conn string) (s *SQLStore, err error) {
	db, err := sqlx.Open("postgres", conn)
	if err != nil {
		return
	}
	s = &SQLStore{
		DB:     db,
		filter: bloom.NewFilter(-1, 0.0001),
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()

	if _, err = tx.Exec(URLSchema); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(CountSchema); err != nil {
		return nil, err
	}
	var cnt int
	if err = tx.QueryRow(
		`SELECT count(*) FROM count`,
	).Scan(&cnt); err != nil {
		return
	} else if cnt == 0 {
		_, err = tx.Exec(
			`INSERT INTO count(url_count, finish_count, error_count, visit_count)
			 VALUES (0, 0, 0, 0)`,
		)
	}
	return
}

func (s *SQLStore) Exist(u *url.URL) (bool, error) {
	return s.filter.Exist(u), nil
}

func (s *SQLStore) GetFunc(u *url.URL, f func(*crawler.URL)) error {
	var w wrapper
	if err := s.DB.QueryRowx(
		`SELECT * FROM url
	    WHERE scheme = $1 AND host = $2 AND path = $3 AND query = $4`,
		u.Scheme, u.Host, u.EscapedPath(), u.Query().Encode(),
	).StructScan(&w); err != nil {
		return err
	}
	f(w.ToURL())
	return nil
}
func (s *SQLStore) Get(u *url.URL) (uu *crawler.URL, err error) {
	err = s.GetFunc(u, func(url *crawler.URL) { uu = url })
	return
}
func (s *SQLStore) GetDepth(u *url.URL) (depth int, err error) {
	err = s.DB.QueryRow(
		`SELECT depth FROM url
    	WHERE scheme = $1 AND host = $2 AND path = $3 AND query = $4`,
		u.Scheme, u.Host, u.EscapedPath(), u.Query().Encode(),
	).Scan(&depth)
	return
}

func (s *SQLStore) GetExtra(u *url.URL) (v interface{}, err error) {
	return nil, errors.New("sqlstore: URL.Extra is not implemented")
}

func (s *SQLStore) PutNX(u *crawler.URL) (ok bool, err error) {
	tx, err := s.DB.Beginx()
	if err != nil {
		return
	}

	done := false
	defer func() {
		if err != nil {
			tx.Rollback() // TODO: handle error
		} else {
			if err = tx.Commit(); err == nil && done {
				s.filter.Add(&u.URL)
				ok = true
			}
		}
	}()

	var cnt int
	if err = tx.QueryRow(`
		SELECT count(*) FROM url
    	WHERE scheme = $1 AND host = $2 AND path = $3 AND query = $4`,
		u.URL.Scheme, u.URL.Host, u.URL.EscapedPath(), u.URL.Query().Encode(),
	).Scan(&cnt); err != nil {
		return
	} else if cnt > 0 {
		return
	}

	w := &wrapper{}
	w.fromURL(u)
	if _, err = tx.NamedExec(`
	INSERT INTO url(scheme, host, path, query, depth, done, status, last, num_visit, num_error)
	 VALUES (:scheme, :host, :path, :query, :depth, :done, :status, :last, :num_visit, :num_error)`,
		w); err == nil {
		done = true
		_, err = tx.Exec(
			`UPDATE count SET url_count = url_count + 1`,
		)
	}
	return
}

func (s *SQLStore) UpdateFunc(u *url.URL, f func(*crawler.URL)) (err error) {
	tx, err := s.DB.Beginx()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()
	var w wrapper
	if err = tx.QueryRowx(
		`SELECT * FROM url
	    WHERE scheme = $1 AND host = $2 AND path = $3 AND query = $4`,
		u.Scheme, u.Host, u.EscapedPath(), u.Query().Encode(),
	).StructScan(&w); err != nil {
		return
	}
	uu := w.ToURL()
	f(uu)
	w.fromURL(uu)
	_, err = s.DB.NamedExec(`
	UPDATE url SET num_error = :num_error, num_visit = :num_visit, last = :last, status = :status
	WHERE scheme = :scheme AND host = :host AND path = :path AND query = :query`, w)
	return

}
func (s *SQLStore) Update(u *crawler.URL) error {
	return s.UpdateFunc(&u.URL, func(uu *crawler.URL) {
		uu.Update(u)
	})
}
func (s *SQLStore) UpdateExtra(u *url.URL, extra interface{}) error {
	return errors.New("sqlstore: URL.Extra is not implemented")
}

func (s *SQLStore) Complete(u *url.URL) (err error) {
	tx, err := s.DB.Beginx()
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			tx.Rollback() // TODO: handle error
		} else {
			err = tx.Commit()
		}
	}()

	if _, err = tx.Exec(`
	UPDATE url SET done = TRUE
	WHERE scheme = $1 AND host = $2 AND path = $3 AND query = $4`,
		u.Scheme,
		u.Host,
		u.EscapedPath(),
		u.Query().Encode(),
	); err != nil {
		return
	}
	_, err = tx.Exec(`UPDATE count SET finish_count = finish_count + 1`)
	return
}

func (s *SQLStore) IncVisitCount() (err error) {
	_, err = s.DB.Exec(
		`UPDATE count SET visit_count = visit_count + 1`,
	)
	return
}
func (s *SQLStore) IncErrorCount() (err error) {
	_, err = s.DB.Exec(
		`UPDATE count SET error_count = error_count + 1`,
	)
	return
}
func (s *SQLStore) IsFinished() (is bool, err error) {
	var rest int
	if err = s.DB.QueryRow(
		`SELECT url_count - finish_count FROM count`,
	).Scan(&rest); err != nil {
		return
	}
	if rest <= 0 {
		is = true
	}
	return
}

func (s *SQLStore) Close() error { return s.DB.Close() }
