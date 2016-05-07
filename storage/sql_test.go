// +build integration

package storage

import (
	"os"
	"testing"

	"github.com/fanyang01/crawler/storage/sqlstore"
)

func TestSQL(t *testing.T) {
	conn := "user=postgres dbname=test sslmode=disable"
	if host := os.Getenv("POSTGRES_PORT_5432_TCP_ADDR"); host != "" {
		conn += " host=" + host
	}
	ss, err := sqlstore.New(conn)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.DB.Close()
	StoreTest(t, ss)
}

func BenchmarkSQLPut(b *testing.B) {
	ss, err := sqlstore.New("user=postgres dbname=test sslmode=disable")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		ss.DB.Exec(`DELETE FROM url`)
		ss.DB.Exec(`DELETE FROM count`)
		ss.DB.Close()
	}()
	benchPut(b, ss, "SQLStore")
}

func BenchmarkSQLGet(b *testing.B) {
	ss, err := sqlstore.New("user=postgres dbname=test sslmode=disable")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		ss.DB.Exec(`DELETE FROM url`)
		ss.DB.Exec(`DELETE FROM count`)
		ss.DB.Close()
	}()
	b.N = 500
	benchGet(b, ss, "SQLStore")
}
