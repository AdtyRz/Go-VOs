package store

import (
	"errors"

	"github.com/go-sql-driver/mysql"
)

func isDuplicate(err error) bool {
	var myErr *mysql.MySQLError
	return errors.As(err, &myErr) && myErr.Number == 1062
}

func isDeadlock(err error) bool {
	var myErr *mysql.MySQLError
	return errors.As(err, &myErr) && (myErr.Number == 1213 || myErr.Number == 1205)
}
