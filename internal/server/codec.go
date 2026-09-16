package server

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nagisa/internal/biz"

	khttp "github.com/go-kratos/kratos/v3/transport/http"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// The netdisk API speaks the canonical protobuf JSON mapping rather than the
// reflection based encoding the kratos default uses. That matters for more
// than taste:
//
//   - enum fields accept and emit their names, so a client reads and writes
//     "PERMISSION_UPLOAD" instead of a bare 3, which is what the generated
//     OpenAPI document describes;
//   - 64 bit integers are strings, which is what the document says and what
//     keeps a byte count beyond 2^53 intact in JavaScript;
//   - bytes fields are base64, and well known types such as Duration and
//     FieldMask use their canonical form;
//   - unknown fields are ignored, so a client that echoes back a full object
//     it just read does not fail on a field the server does not know.
//
// Decoding stays a superset of the old behaviour: protojson also accepts the
// numeric form of an enum and the numeric form of a 64 bit integer, so an
// existing client keeps working unchanged.

// ProtoJSONRequestDecoder decodes a request body as protobuf JSON.
func ProtoJSONRequestDecoder(r *http.Request, v any) error {
	msg, ok := v.(proto.Message)
	if !ok {
		return khttp.DefaultRequestDecoder(r, v)
	}
	if r.Body == nil {
		return nil
	}
	if contentType := r.Header.Get("Content-Type"); contentType != "" &&
		!strings.Contains(contentType, "json") && !strings.Contains(contentType, "form") {
		return khttp.DefaultRequestDecoder(r, v)
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		// A body over the configured limit is reported as too large instead of
		// as an opaque server error.
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return biz.ErrTooLarge.WithMetadata(map[string]string{
				"limit_bytes": strconv.FormatInt(tooLarge.Limit, 10),
			})
		}
		return fmt.Errorf("body read: %w", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, msg); err != nil {
		return fmt.Errorf("body unmarshal json: %w", err)
	}
	return nil
}

// ProtoJSONResponseEncoder writes a reply as protobuf JSON.
func ProtoJSONResponseEncoder(w http.ResponseWriter, r *http.Request, v any) error {
	msg, ok := v.(proto.Message)
	if !ok {
		return khttp.DefaultResponseEncoder(w, r, v)
	}
	raw, err := (protojson.MarshalOptions{}).Marshal(msg)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(raw)
	return err
}

// ProtoJSONQueryDecoder binds the query string onto a proto request message.
//
// The kratos default binds through the generated struct tags, which carry the
// original proto field name, so a client would have to send `page_size` where
// the OpenAPI document advertises `pageSize`. This decoder accepts both, which
// keeps the document accurate without breaking a snake_case caller.
func ProtoJSONQueryDecoder(r *http.Request, v any) error {
	msg, ok := v.(proto.Message)
	if !ok {
		return khttp.DefaultRequestQuery(r, v)
	}
	query := r.URL.Query()
	if len(query) == 0 {
		return nil
	}
	fields := msg.ProtoReflect().Descriptor().Fields()
	index := make(map[string]protoreflect.FieldDescriptor, fields.Len()*2)
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		index[strings.ToLower(field.JSONName())] = field
		index[strings.ToLower(string(field.Name()))] = field
	}
	target := msg.ProtoReflect()
	for key, raw := range query {
		field, ok := index[strings.ToLower(key)]
		if !ok || len(raw) == 0 || raw[0] == "" {
			continue
		}
		if field.IsList() || field.IsMap() {
			return fmt.Errorf("query parameter %q cannot be bound from the query string", key)
		}
		if err := setScalarField(target, field, raw[0]); err != nil {
			return fmt.Errorf("query parameter %q: %w", key, err)
		}
	}
	return nil
}

// setScalarField converts one query value onto a field.
func setScalarField(msg protoreflect.Message, field protoreflect.FieldDescriptor, raw string) error {
	switch field.Kind() {
	case protoreflect.BoolKind:
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("expected a boolean, got %q", raw)
		}
		msg.Set(field, protoreflect.ValueOfBool(value))
	case protoreflect.EnumKind:
		if named := field.Enum().Values().ByName(protoreflect.Name(raw)); named != nil {
			msg.Set(field, protoreflect.ValueOfEnum(named.Number()))
			return nil
		}
		number, err := strconv.ParseInt(raw, 10, 32)
		if err != nil {
			return fmt.Errorf("expected an enum name or number, got %q", raw)
		}
		msg.Set(field, protoreflect.ValueOfEnum(protoreflect.EnumNumber(number)))
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		value, err := strconv.ParseInt(raw, 10, 32)
		if err != nil {
			return fmt.Errorf("expected an integer, got %q", raw)
		}
		msg.Set(field, protoreflect.ValueOfInt32(int32(value)))
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("expected an integer, got %q", raw)
		}
		msg.Set(field, protoreflect.ValueOfInt64(value))
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		value, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			return fmt.Errorf("expected an unsigned integer, got %q", raw)
		}
		msg.Set(field, protoreflect.ValueOfUint32(uint32(value)))
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("expected an unsigned integer, got %q", raw)
		}
		msg.Set(field, protoreflect.ValueOfUint64(value))
	case protoreflect.FloatKind:
		value, err := strconv.ParseFloat(raw, 32)
		if err != nil {
			return fmt.Errorf("expected a number, got %q", raw)
		}
		msg.Set(field, protoreflect.ValueOfFloat32(float32(value)))
	case protoreflect.DoubleKind:
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return fmt.Errorf("expected a number, got %q", raw)
		}
		msg.Set(field, protoreflect.ValueOfFloat64(value))
	case protoreflect.StringKind:
		msg.Set(field, protoreflect.ValueOfString(raw))
	case protoreflect.BytesKind:
		decoded, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return fmt.Errorf("expected base64, got %q", raw)
		}
		msg.Set(field, protoreflect.ValueOfBytes(decoded))
	case protoreflect.MessageKind:
		// The only message typed query parameters in this API are timestamps.
		if field.Message().FullName() != "google.protobuf.Timestamp" {
			return fmt.Errorf("message typed parameter %q cannot be bound", field.Name())
		}
		ts, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return fmt.Errorf("expected an RFC 3339 timestamp, got %q", raw)
		}
		holder := msg.NewField(field).Message()
		holder.Set(holder.Descriptor().Fields().ByName("seconds"), protoreflect.ValueOfInt64(ts.Unix()))
		holder.Set(holder.Descriptor().Fields().ByName("nanos"), protoreflect.ValueOfInt32(int32(ts.Nanosecond())))
		msg.Set(field, protoreflect.ValueOfMessage(holder))
	default:
		return fmt.Errorf("unsupported parameter kind %s", field.Kind())
	}
	return nil
}
