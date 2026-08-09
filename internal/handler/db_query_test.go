package handler

import (
	"database/sql/driver"
	"errors"
	"testing"
)

func TestIsRetryableQueryErr(t *testing.T) {
	if !isRetryableQueryErr(driver.ErrBadConn) {
		t.Fatal("expected ErrBadConn to be retryable")
	}
	if isRetryableQueryErr(errors.New("syntax error near")) {
		t.Fatal("syntax errors must not be retried")
	}
	if isRetryableQueryErr(errors.New("read tcp 127.0.0.1:3306: i/o timeout")) {
		t.Fatal("i/o timeout must not be retried")
	}
}
