package timescale

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakePool is an in-memory stand-in for *pgxpool.Pool used by historian unit tests.
type fakePool struct {
	mu sync.Mutex

	pingErr error
	closed  bool

	execFn     func(sql string, args []any) (pgconn.CommandTag, error)
	queryFn    func(sql string, args []any) (pgx.Rows, error)
	queryRowFn func(sql string, args []any) pgx.Row

	execCalls     []string
	queryCalls    []string
	queryRowCalls []string
}

func (p *fakePool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
}

func (p *fakePool) Ping(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.pingErr
}

func (p *fakePool) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	p.mu.Lock()
	p.execCalls = append(p.execCalls, sql)
	fn := p.execFn
	p.mu.Unlock()
	if fn != nil {
		return fn(sql, arguments)
	}
	return pgconn.NewCommandTag("OK"), nil
}

func (p *fakePool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	p.mu.Lock()
	p.queryCalls = append(p.queryCalls, sql)
	fn := p.queryFn
	p.mu.Unlock()
	if fn != nil {
		return fn(sql, args)
	}
	return &fakeRows{}, nil
}

func (p *fakePool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	p.mu.Lock()
	p.queryRowCalls = append(p.queryRowCalls, sql)
	fn := p.queryRowFn
	p.mu.Unlock()
	if fn != nil {
		return fn(sql, args)
	}
	return fakeRow{err: errFakeNoRow}
}

var errFakeNoRow = errString("fake: no QueryRow handler")

type errString string

func (e errString) Error() string { return string(e) }

type fakeRow struct {
	scan func(dest ...any) error
	err  error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if r.scan != nil {
		return r.scan(dest...)
	}
	return nil
}

func rowInt64(v int64) pgx.Row {
	return fakeRow{scan: func(dest ...any) error {
		*(dest[0].(*int64)) = v
		return nil
	}}
}

func rowString(v string) pgx.Row {
	return fakeRow{scan: func(dest ...any) error {
		*(dest[0].(*string)) = v
		return nil
	}}
}

func rowErr(err error) pgx.Row {
	return fakeRow{err: err}
}

func rowTimes(oldest, newest time.Time) pgx.Row {
	return fakeRow{scan: func(dest ...any) error {
		o := oldest
		n := newest
		*(dest[0].(**time.Time)) = &o
		*(dest[1].(**time.Time)) = &n
		return nil
	}}
}

func rowTimePtr(t *time.Time) pgx.Row {
	return fakeRow{scan: func(dest ...any) error {
		*(dest[0].(**time.Time)) = t
		return nil
	}}
}

type fakeRows struct {
	vals   [][]any
	i      int
	err    error
	closed bool
}

func (r *fakeRows) Close() { r.closed = true }

func (r *fakeRows) Err() error { return r.err }

func (r *fakeRows) CommandTag() pgconn.CommandTag { return pgconn.NewCommandTag("") }

func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (r *fakeRows) Next() bool {
	if r.i >= len(r.vals) {
		r.closed = true
		return false
	}
	r.i++
	return true
}

func (r *fakeRows) Scan(dest ...any) error {
	if r.i == 0 || r.i > len(r.vals) {
		return errString("fakeRows: Scan before Next")
	}
	row := r.vals[r.i-1]
	for i, d := range dest {
		if i >= len(row) {
			break
		}
		if err := assignScan(d, row[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *fakeRows) Values() ([]any, error) { return nil, nil }

func (r *fakeRows) RawValues() [][]byte { return nil }

func (r *fakeRows) Conn() *pgx.Conn { return nil }

func assignScan(dest, src any) error {
	switch d := dest.(type) {
	case *time.Time:
		v, ok := src.(time.Time)
		if !ok {
			return errString("fakeRows: bad time")
		}
		*d = v
	case *string:
		v, ok := src.(string)
		if !ok {
			return errString("fakeRows: bad string")
		}
		*d = v
	case **float64:
		if src == nil {
			*d = nil
			return nil
		}
		v, ok := src.(float64)
		if !ok {
			return errString("fakeRows: bad float64")
		}
		*d = &v
	case **string:
		if src == nil {
			*d = nil
			return nil
		}
		v, ok := src.(string)
		if !ok {
			return errString("fakeRows: bad *string")
		}
		*d = &v
	case **bool:
		if src == nil {
			*d = nil
			return nil
		}
		v, ok := src.(bool)
		if !ok {
			return errString("fakeRows: bad bool")
		}
		*d = &v
	case *int:
		v, ok := src.(int)
		if !ok {
			return errString("fakeRows: bad int")
		}
		*d = v
	case *int64:
		v, ok := src.(int64)
		if !ok {
			return errString("fakeRows: bad int64")
		}
		*d = v
	default:
		return errString("fakeRows: unsupported dest")
	}
	return nil
}

func sqlHas(sql string, parts ...string) bool {
	s := strings.ToLower(sql)
	for _, p := range parts {
		if !strings.Contains(s, strings.ToLower(p)) {
			return false
		}
	}
	return true
}
