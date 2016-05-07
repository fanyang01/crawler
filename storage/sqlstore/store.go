package sqlstore

import (
	"database/sql"
	"net/url"

	"github.com/fanyang01/crawler"
	"github.com/fanyang01/crawler/bloom"
)

type SQLStore struct {
	DB     *sql.DB
	filter *bloom.Filter
}

const (
	urlSchema = `
CREATE TABLE IF NOT EXISTS url (
	scheme TEXT,
	host TEXT,
	path TEXT,
	query TEXT,
	depth INT,
	status INT,
	freq NUMERIC,
	last TIMESTAMP,
	visit_count INT,
	err_count INT,
	PRIMARY KEY (scheme, host, path, query)
)`
	countSchema = `
CREATE TABLE IF NOT EXISTS count (
	url_count INT,
	finish_count INT,
	error_count INT,
	visit_count INT
)
`
)

func New(driver, uri string) (s *SQLStore, err error) {
	db, err := sql.Open(driver, uri)
	if err != nil {
		return
	}
	tx, err := db.Begin()
	if err != nil {
		return
	}
	s = &SQLStore{
		DB:     db,
		filter: bloom.NewFilter(-1, 0.0001),
	}
	defer func() {
		if err != nil {
			tx.Rollback() // TODO
		} else {
			err = tx.Commit()
		}
	}()

	if _, err = tx.Exec(urlSchema); err != nil {
		return
	}
	if _, err = tx.Exec(countSchema); err != nil {
		return
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
func (s *SQLStore) Get(u *url.URL) (uu *crawler.URL, err error) {
	uu = &crawler.URL{}
	err = s.DB.QueryRow(
		`SELECT scheme, host, path, query, depth, status, last, visit_count, err_count
    	FROM url
    	WHERE scheme = $1 AND host = $2 AND path = $3 AND query = $4`,
		u.Scheme, u.Host, u.Path, u.RawQuery,
	).Scan(
		&uu.URL.Scheme,
		&uu.URL.Host,
		&uu.URL.Path,
		&uu.URL.RawQuery,
		&uu.Depth,
		&uu.Status,
		&uu.Last,
		&uu.NumVisit,
		&uu.NumError,
	)
	return
}

func (s *SQLStore) GetDepth(u *url.URL) (depth int, err error) {
	err = s.DB.QueryRow(
		`SELECT depth FROM url
    	WHERE scheme = $1 AND host = $2 AND path = $3 AND query = $4`,
		u.Scheme, u.Host, u.Path, u.RawQuery,
	).Scan(&depth)
	return
}
func (s *SQLStore) PutNX(u *crawler.URL) (ok bool, err error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return
	}

	put := false
	defer func() {
		if err != nil {
			tx.Rollback() // TODO: handle error
		} else {
			if err = tx.Commit(); err == nil && put {
				s.filter.Add(&u.URL)
				ok = true
			}
		}
	}()

	var cnt int
	if err = tx.QueryRow(`
		SELECT count(*) FROM url
    	WHERE scheme = $1 AND host = $2 AND path = $3 AND query = $4`,
		u.URL.Scheme, u.URL.Host, u.URL.Path, u.URL.RawQuery,
	).Scan(&cnt); err != nil {
		return
	} else if cnt > 0 {
		return
	}

	if _, err = tx.Exec(`
	INSERT INTO url(scheme, host, path, query, depth, status, last, visit_count, err_count)
	 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		u.URL.Scheme,
		u.URL.Host,
		u.URL.Path,
		u.URL.RawQuery,
		u.Depth,
		u.Status,
		u.Last,
		u.NumVisit,
		u.NumError,
	); err == nil {
		put = true
		_, err = tx.Exec(
			`UPDATE count SET url_count = url_count + 1`,
		)
	}
	return
}
func (s *SQLStore) Update(u *crawler.URL) (err error) {
	_, err = s.DB.Exec(`
	UPDATE url SET err_count = $1, visit_count = $2, last = $3 
	WHERE scheme = $4 AND host = $5 AND path = $6 AND query = $7`,
		u.NumError,
		u.NumVisit,
		u.Last,

		u.URL.Scheme,
		u.URL.Host,
		u.URL.Path,
		u.URL.RawQuery,
	)
	return
}
func (s *SQLStore) UpdateStatus(u *url.URL, status int) (err error) {
	tx, err := s.DB.Begin()
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
	UPDATE url SET status = $1
	WHERE scheme = $2 AND host = $3 AND path = $4 AND query = $5`,
		status,

		u.Scheme,
		u.Host,
		u.Path,
		u.RawQuery,
	); err != nil {
		return
	}
	switch status {
	case crawler.URLStatusFinished, crawler.URLStatusError:
		_, err = tx.Exec(
			`UPDATE count SET finish_count = finish_count + 1`,
		)
	}
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
