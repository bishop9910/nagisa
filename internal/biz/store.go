package biz

import (
	"context"

	"go.einride.tech/aip/filtering"
	"go.einride.tech/aip/ordering"
)

// TxManager runs a function inside a single database transaction. Every
// repository call made with the context it passes down joins that transaction,
// so a usecase can span several repositories and still commit atomically.
type TxManager interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// ListOption configures a list query.
type ListOption func(*ListOptions)

// ListOptions are the query options shared by every list operation.
type ListOptions struct {
	Filter  filtering.Filter
	OrderBy ordering.OrderBy
	Offset  int
	Limit   int
}

// ListFilter sets a standard AIP filter.
func ListFilter(filter filtering.Filter) ListOption {
	return func(o *ListOptions) {
		o.Filter = filter
	}
}

// ListOrderBy sets a standard AIP order_by value.
func ListOrderBy(orderBy ordering.OrderBy) ListOption {
	return func(o *ListOptions) {
		o.OrderBy = orderBy
	}
}

// ListOffset sets an offset.
func ListOffset(offset int) ListOption {
	return func(o *ListOptions) {
		o.Offset = offset
	}
}

// ListLimit sets a limit.
func ListLimit(limit int) ListOption {
	return func(o *ListOptions) {
		o.Limit = limit
	}
}

// ResolveListOptions folds the options into a concrete query description.
func ResolveListOptions(defaultLimit int, opts ...ListOption) ListOptions {
	o := ListOptions{Limit: defaultLimit}
	for _, opt := range opts {
		opt(&o)
	}
	if o.Limit <= 0 {
		o.Limit = defaultLimit
	}
	if o.Offset < 0 {
		o.Offset = 0
	}
	return o
}
