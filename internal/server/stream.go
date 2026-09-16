package server

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	v1 "nagisa/api/netdisk/v1"
	"nagisa/internal/biz"

	khttp "github.com/go-kratos/kratos/v3/transport/http"
	"github.com/google/uuid"
)

// StreamHandler serves the binary endpoints that a browser cannot express
// through the proto JSON codec: original content with byte ranges, on the fly
// zip archives and raw part payloads.
//
// These routes are registered beside the generated ones and authorise
// themselves: the streaming reads rely on the signature minted by
// GetDownloadUrl and GetArchiveUrl, the part upload relies on the bearer token
// of the owning session.
type StreamHandler struct {
	files  *biz.FileUsecase
	nodes  *biz.NodeUsecase
	auth   *biz.AuthUsecase
	signer biz.URLSigner
}

// NewStreamHandler returns the raw endpoint handler.
func NewStreamHandler(files *biz.FileUsecase, nodes *biz.NodeUsecase, auth *biz.AuthUsecase, signer biz.URLSigner) *StreamHandler {
	return &StreamHandler{files: files, nodes: nodes, auth: auth, signer: signer}
}

// Content streams the original bytes of a file. It honours a Range header so a
// download can be resumed, and it is the endpoint a signed download URL points
// at when the client cannot reach object storage directly.
func (h *StreamHandler) Content(ctx khttp.Context) error {
	nodeID, err := uuid.Parse(ctx.Vars().Get("node_id"))
	if err != nil {
		return biz.ErrInvalidArgument
	}
	path := "/v1/files/" + nodeID.String() + "/content"
	if err := h.verify(ctx, "content", nodeID, path); err != nil {
		return err
	}
	node, err := h.files.NodeByID(ctx, nodeID)
	if err != nil {
		return err
	}
	if node.Kind != biz.NodeKindFile || node.StorageKey == "" {
		return biz.ErrUnsupported
	}
	disposition := "attachment"
	if extra := ctx.Query().Get("extra"); extra == "inline" {
		disposition = "inline"
	}
	offset, length, partial, err := parseRange(ctx.Request().Header.Get("Range"), node.Size)
	if err != nil {
		return err
	}
	reader, info, err := h.files.Store().GetObject(ctx, node.StorageKey, offset, length)
	if err != nil {
		return err
	}
	defer func() {
		_ = reader.Close()
	}()
	header := ctx.Response().Header()
	header.Set("Content-Type", node.MimeType)
	header.Set("Content-Disposition", contentDisposition(disposition, node.Name))
	header.Set("Accept-Ranges", "bytes")
	header.Set("ETag", strconv.Quote(node.Etag))
	if partial {
		total := node.Size
		if info != nil && info.Size > 0 && total == 0 {
			total = info.Size
		}
		end := offset + length - 1
		if length <= 0 {
			end = total - 1
		}
		header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, end, total))
		header.Set("Content-Length", strconv.FormatInt(end-offset+1, 10))
		ctx.Response().WriteHeader(http.StatusPartialContent)
	} else if node.Size > 0 {
		header.Set("Content-Length", strconv.FormatInt(node.Size, 10))
	}
	_, err = io.Copy(ctx.Response(), reader)
	if errors.Is(err, io.ErrClosedPipe) || errors.Is(err, io.EOF) {
		return nil
	}
	return nil
}

// Archive streams a folder, or a single file, as a zip archive. The archive is
// produced on the fly, so a folder of any size can be downloaded without
// staging a temporary object.
func (h *StreamHandler) Archive(ctx khttp.Context) error {
	nodeID, err := uuid.Parse(ctx.Vars().Get("node_id"))
	if err != nil {
		return biz.ErrInvalidArgument
	}
	path := "/v1/files/" + nodeID.String() + "/archive"
	if err := h.verify(ctx, "archive", nodeID, path); err != nil {
		return err
	}
	root, err := h.files.NodeByID(ctx, nodeID)
	if err != nil {
		return err
	}
	name := ctx.Query().Get("extra")
	if name == "" {
		name = root.Name
	}
	header := ctx.Response().Header()
	header.Set("Content-Type", "application/zip")
	header.Set("Content-Disposition", contentDisposition("attachment", name+".zip"))
	ctx.Response().WriteHeader(http.StatusOK)

	archive := zip.NewWriter(ctx.Response())
	defer func() {
		_ = archive.Close()
	}()
	if root.Kind == biz.NodeKindFile {
		return h.writeArchiveEntry(ctx, archive, root, root.Name)
	}
	children, err := h.files.ListSubtree(ctx, root.ID)
	if err != nil {
		return err
	}
	prefix := strings.TrimSuffix(root.Name, "/") + "/"
	for _, child := range children {
		rel := h.relativePath(ctx, root, child)
		if child.Kind == biz.NodeKindFolder {
			if _, err := archive.Create(prefix + rel + "/"); err != nil {
				return nil
			}
			continue
		}
		if err := h.writeArchiveEntry(ctx, archive, child, prefix+rel); err != nil {
			return nil
		}
	}
	return nil
}

func (h *StreamHandler) writeArchiveEntry(ctx khttp.Context, archive *zip.Writer, node *biz.Node, name string) error {
	entry, err := archive.CreateHeader(&zip.FileHeader{
		Name:     name,
		Method:   zip.Deflate,
		Modified: node.UpdatedAt,
	})
	if err != nil {
		return err
	}
	if node.StorageKey == "" {
		return nil
	}
	reader, _, err := h.files.Store().GetObject(ctx, node.StorageKey, 0, 0)
	if err != nil {
		return err
	}
	defer func() {
		_ = reader.Close()
	}()
	_, err = io.Copy(entry, reader)
	return err
}

// relativePath renders a node's location relative to the archive root.
func (h *StreamHandler) relativePath(ctx context.Context, root, node *biz.Node) string {
	display, err := h.nodes.DisplayPath(ctx, node)
	if err != nil {
		return node.Name
	}
	rootDisplay, err := h.nodes.DisplayPath(ctx, root)
	if err != nil {
		return node.Name
	}
	rel := strings.TrimPrefix(display, rootDisplay)
	return strings.TrimPrefix(rel, "/")
}

// UploadRawChunk stores one part whose payload is the raw request body. It
// exists so a browser can stream a slice of a file without base64 encoding it,
// which would inflate the transfer by a third.
func (h *StreamHandler) UploadRawChunk(ctx khttp.Context) error {
	uploadID, err := uuid.Parse(ctx.Vars().Get("upload_id"))
	if err != nil {
		return biz.ErrInvalidArgument
	}
	partNumber, err := strconv.ParseInt(ctx.Vars().Get("part_number"), 10, 32)
	if err != nil || partNumber < 1 {
		return biz.ErrInvalidArgument
	}
	caller, err := h.authenticate(ctx)
	if err != nil {
		return err
	}
	requestCtx := biz.NewContext(ctx, caller)
	size := ctx.Request().ContentLength
	if size < 0 {
		return biz.ErrInvalidArgument
	}
	if _, _, err := h.files.UploadChunk(requestCtx, biz.UploadChunkInput{
		UploadID:   uploadID,
		PartNumber: int32(partNumber),
		Size:       size,
		Reader:     http.MaxBytesReader(ctx.Response(), ctx.Request().Body, size),
	}); err != nil {
		return err
	}
	return ctx.Result(http.StatusOK, &v1.UploadPart{PartNumber: int32(partNumber), Size: size})
}

// verify checks the signature of a streaming read.
func (h *StreamHandler) verify(ctx khttp.Context, scope string, nodeID uuid.UUID, path string) error {
	if h.signer == nil {
		return biz.ErrUnsupported
	}
	query := ctx.Request().URL.Query()
	extra := query.Get("extra")
	if err := h.signer.Verify(
		scope, http.MethodGet, path, nodeID.String(), extra,
		query.Get("exp"), query.Get("sig"), query.Get("sub"), time.Now(),
	); err != nil {
		return biz.ErrPermissionDenied
	}
	return nil
}

// authenticate resolves the bearer token of a raw request.
func (h *StreamHandler) authenticate(ctx khttp.Context) (*biz.Caller, error) {
	token := bearer(ctx.Request().Header.Get("Authorization"))
	if token == "" {
		return nil, biz.ErrUnauthenticated
	}
	caller, err := h.auth.Authenticate(ctx, token)
	if err != nil {
		return nil, err
	}
	caller.IP = ctx.Request().RemoteAddr
	caller.UserAgent = ctx.Request().UserAgent()
	caller.Unlocked = map[uuid.UUID]bool{}
	if unlock := ctx.Request().Header.Get("X-Node-Token"); unlock != "" {
		if ids, err := h.auth.ParseNodeToken(unlock); err == nil {
			for _, id := range ids {
				caller.Unlocked[id] = true
			}
		}
	}
	return caller, nil
}

func bearer(header string) string {
	if len(header) > 7 && strings.EqualFold(header[:7], "bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return strings.TrimSpace(header)
}

// parseRange turns a Range header into an offset and a length. It reports
// whether the client asked for a partial response.
func parseRange(header string, size int64) (int64, int64, bool, error) {
	header = strings.TrimSpace(header)
	if header == "" || !strings.HasPrefix(header, "bytes=") {
		return 0, 0, false, nil
	}
	spec := strings.TrimPrefix(header, "bytes=")
	if strings.Contains(spec, ",") {
		// Multi range responses are not worth the complexity for a netdisk.
		return 0, 0, false, nil
	}
	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 {
		return 0, 0, false, biz.ErrInvalidArgument
	}
	startRaw, endRaw := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	switch {
	case startRaw == "" && endRaw == "":
		return 0, 0, false, biz.ErrInvalidArgument
	case startRaw == "":
		suffix, err := strconv.ParseInt(endRaw, 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, false, biz.ErrInvalidArgument
		}
		if size > 0 && suffix > size {
			suffix = size
		}
		return size - suffix, suffix, true, nil
	default:
		start, err := strconv.ParseInt(startRaw, 10, 64)
		if err != nil || start < 0 {
			return 0, 0, false, biz.ErrInvalidArgument
		}
		if endRaw == "" {
			if size > 0 {
				return start, size - start, true, nil
			}
			return start, 0, true, nil
		}
		end, err := strconv.ParseInt(endRaw, 10, 64)
		if err != nil || end < start {
			return 0, 0, false, biz.ErrInvalidArgument
		}
		return start, end - start + 1, true, nil
	}
}

// contentDisposition is duplicated from the storage layer on purpose: the
// server must be able to render a header without depending on the storage
// client's internals.
func contentDisposition(disposition, fileName string) string {
	if disposition == "" {
		disposition = "attachment"
	}
	ascii := strings.Map(func(r rune) rune {
		if r < 32 || r > 126 || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, fileName)
	return fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`, disposition, ascii, urlEscape(fileName))
}

// urlEscape percent encodes a file name for the RFC 6266 extended form.
func urlEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}
