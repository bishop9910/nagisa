package service

import (
	"bytes"
	"io"
	"regexp"
	"strings"
	"time"

	"nagisa/internal/biz"

	"go.einride.tech/aip/filtering"
	"go.einride.tech/aip/ordering"
	"go.einride.tech/aip/pagination"
	"google.golang.org/protobuf/proto"
)

// Page size bounds shared by every list endpoint.
const (
	defaultPageSize = 20
	maxPageSize     = 1000
	// GetNodeTree bounds each folder separately rather than the whole reply.
	defaultTreePageSize = 200
	maxTreePageSize     = 1000
)

// pagedRequest is the pagination contract. Every AIP list request message
// satisfies it, including the ones that expose neither a filter nor an
// ordering field.
type pagedRequest interface {
	proto.Message
	GetPageSize() int32
	GetPageToken() string
}

// orderedRequest adds the ordering field.
type orderedRequest interface {
	pagedRequest
	GetOrderBy() string
}

// listRequest adds the filter field.
type listRequest interface {
	orderedRequest
	GetFilter() string
}

// nowFunc is the clock used for reply only computations, kept as a variable so
// tests can pin it.
var nowFunc = time.Now

// filterStringLiteral matches a double quoted string literal, escapes included,
// so a rewrite never touches the inside of a value.
var filterStringLiteral = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)

// normalizeBoolLiterals rewrites boolean comparisons into the form the AIP
// parser understands.
//
// The AIP grammar has no boolean literals: `true` and `false` are lexed as
// identifiers, so `filter=success=true` fails to resolve. A bare identifier is
// the documented way to match a boolean field, therefore:
//
//	field=true   -> field          field=false  -> NOT field
//	field!=true  -> NOT field      field!=false -> field
//
// Only the field names passed in are rewritten; every other expression, and
// everything inside a quoted value, is left untouched.
func normalizeBoolLiterals(filter string, fields ...string) string {
	if filter == "" || len(fields) == 0 {
		return filter
	}
	quoted := make([]string, 0, len(fields))
	for _, field := range fields {
		quoted = append(quoted, regexp.QuoteMeta(field))
	}
	pattern := regexp.MustCompile(`(?i)\b(` + strings.Join(quoted, "|") + `)\s*(!?=)\s*(true|false)\b`)
	rewrite := func(segment string) string {
		return pattern.ReplaceAllStringFunc(segment, func(match string) string {
			parts := pattern.FindStringSubmatch(match)
			// parts[0] is the whole match, then field, operator and literal.
			if len(parts) < 4 {
				return match
			}
			field, operator, literal := parts[1], parts[2], strings.ToLower(parts[3])
			if (operator == "=") == (literal == "false") {
				return "NOT " + field
			}
			return field
		})
	}

	var out strings.Builder
	last := 0
	for _, loc := range filterStringLiteral.FindAllStringIndex(filter, -1) {
		out.WriteString(rewrite(filter[last:loc[0]]))
		out.WriteString(filter[loc[0]:loc[1]])
		last = loc[1]
	}
	out.WriteString(rewrite(filter[last:]))
	return out.String()
}

// parseList turns a full AIP list request into domain list options.
func parseList[T listRequest](req T, declarations *filtering.Declarations, maxSize int, orderPaths ...string) ([]biz.ListOption, int32, pagination.PageToken, error) {
	filter, err := filtering.ParseFilter(req, declarations)
	if err != nil {
		return nil, 0, pagination.PageToken{}, biz.ErrInvalidArgument
	}
	return parseOrdered(req, maxSize, orderPaths, biz.ListFilter(filter))
}

// parseOrderedList parses a request that supports ordering but not filtering.
func parseOrderedList[T orderedRequest](req T, maxSize int, orderPaths ...string) ([]biz.ListOption, int32, pagination.PageToken, error) {
	return parseOrdered(req, maxSize, orderPaths)
}

// parsePagedList parses a request that only supports pagination.
func parsePagedList[T pagedRequest](req T, maxSize int) ([]biz.ListOption, int32, pagination.PageToken, error) {
	return parseOrdered[pagedRequest](req, maxSize, nil)
}

// parseOrdered performs the shared validation of a list request.
func parseOrdered[T pagedRequest](req T, maxSize int, orderPaths []string, extra ...biz.ListOption) ([]biz.ListOption, int32, pagination.PageToken, error) {
	if maxSize <= 0 {
		maxSize = maxPageSize
	}
	pageToken, err := pagination.ParsePageToken(req)
	if err != nil {
		return nil, 0, pagination.PageToken{}, biz.ErrInvalidArgument
	}
	size := req.GetPageSize()
	if size <= 0 {
		size = defaultPageSize
	}
	if size > int32(maxSize) {
		size = int32(maxSize)
	}
	options := make([]biz.ListOption, 0, 5)
	options = append(options, extra...)
	if ordered, ok := any(req).(orderedRequest); ok && len(orderPaths) > 0 {
		orderBy, err := ordering.ParseOrderBy(ordered)
		if err != nil {
			return nil, 0, pagination.PageToken{}, biz.ErrInvalidArgument
		}
		if err := orderBy.ValidateForPaths(orderPaths...); err != nil {
			return nil, 0, pagination.PageToken{}, biz.ErrInvalidArgument
		}
		options = append(options, biz.ListOrderBy(orderBy))
	}
	options = append(options,
		biz.ListLimit(int(size)),
		biz.ListOffset(int(pageToken.Offset)),
	)
	return options, size, pageToken, nil
}

// nextPageToken returns the token of the following page, or an empty string
// when the current page was not full.
func nextPageToken[T pagedRequest](req T, token pagination.PageToken, returned, size int) string {
	if returned < size {
		return ""
	}
	return token.Next(req).String()
}

// declarations builds the AIP filter declarations for a resource.
func declarations(opts ...filtering.DeclarationOption) (*filtering.Declarations, error) {
	options := append([]filtering.DeclarationOption{filtering.DeclareStandardFunctions()}, opts...)
	d, err := filtering.NewDeclarations(options...)
	if err != nil {
		return nil, biz.ErrInternal
	}
	return d, nil
}

// timeOrZero dereferences an optional timestamp.
func timeOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// timeUntil returns the remaining time before an instant.
func timeUntil(t time.Time) time.Duration {
	d := time.Until(t)
	if d < 0 {
		return 0
	}
	return d
}

// bytesReader wraps a byte slice so a streaming upload can consume it without
// copying the payload again.
func bytesReader(b []byte) io.Reader {
	return bytes.NewReader(b)
}
