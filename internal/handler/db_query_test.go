package handler

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestIsRetryableQueryErr(t *testing.T) {
	if !isRetryableQueryErr(driver.ErrBadConn) {
		t.Fatal("expected ErrBadConn to be retryable")
	}
	if isRetryableQueryErr(errors.New("syntax error near")) {
		t.Fatal("syntax errors must not be retried")
	}
	if !isRetryableQueryErr(errors.New("read tcp 127.0.0.1:3306: i/o timeout")) {
		t.Fatal("expected i/o timeout to be retryable")
	}
}

func TestIsFTSCacheLimitErr(t *testing.T) {
	if !isFTSCacheLimitErr(&mysql.MySQLError{Number: 188, Message: "FTS query exceeds result cache limit"}) {
		t.Fatal("expected MySQL error 188 to match")
	}
	if isFTSCacheLimitErr(fmt.Errorf("syntax error")) {
		t.Fatal("syntax errors must not match")
	}
}
