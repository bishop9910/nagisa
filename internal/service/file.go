package service

import (
	"context"

	v1 "nagisa/api/netdisk/v1"
	"nagisa/internal/biz"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"
)

// FileService implements content transfer: resumable multipart upload, signed
// download URLs and previews.
type FileService struct {
	v1.UnimplementedFileServiceServer

	uc    *biz.FileUsecase
	nodes *biz.NodeUsecase
	users *biz.UserUsecase
}

// NewFileService new a file service.
func NewFileService(uc *biz.FileUsecase, nodes *biz.NodeUsecase, users *biz.UserUsecase) *FileService {
	return &FileService{uc: uc, nodes: nodes, users: users}
}

// InitiateUpload opens a multipart upload session.
func (s *FileService) InitiateUpload(ctx context.Context, req *v1.InitiateUploadRequest) (*v1.UploadSession, error) {
	parentID, err := parseOptionalUUID(req.GetParentId())
	if err != nil {
		return nil, err
	}
	resumeID, err := parseOptionalUUID(req.GetResumableUploadId())
	if err != nil {
		return nil, err
	}
	upload, parts, err := s.uc.InitiateUpload(ctx, biz.InitiateUploadInput{
		ParentID:    parentID,
		Name:        req.GetName(),
		Size:        req.GetSize(),
		MimeType:    req.GetMimeType(),
		ChunkSize:   req.GetChunkSize(),
		Mode:        parseUploadMode(req.GetMode()),
		Policy:      parseConflictPolicy(req.GetConflictPolicy()),
		Etag:        req.GetEtag(),
		Description: req.GetDescription(),
		Metadata:    req.GetMetadata(),
		ResumeID:    resumeID,
	})
	if err != nil {
		return nil, err
	}
	return convertUpload(upload, parts), nil
}

// ListUploads returns the caller's upload sessions.
func (s *FileService) ListUploads(ctx context.Context, req *v1.ListUploadsRequest) (*v1.UploadSessionSet, error) {
	opts, size, token, err := parsePagedList(req, 200)
	if err != nil {
		return nil, err
	}
	parentID, err := parseOptionalUUID(req.GetParentId())
	if err != nil {
		return nil, err
	}
	uploads, total, err := s.uc.ListUploads(ctx, parseUploadStatus(req.GetStatus()), parentID, opts...)
	if err != nil {
		return nil, err
	}
	set := &v1.UploadSessionSet{TotalSize: total, Uploads: make([]*v1.UploadSession, 0, len(uploads))}
	for _, u := range uploads {
		set.Uploads = append(set.Uploads, convertUpload(u, nil))
	}
	set.NextPageToken = nextPageToken(req, token, len(uploads), int(size))
	return set, nil
}

// CompleteUpload verifies the parts and commits the node.
func (s *FileService) CompleteUpload(ctx context.Context, req *v1.CompleteUploadRequest) (*v1.Node, error) {
	uploadID, err := parseUUID(req.GetUploadId())
	if err != nil {
		return nil, err
	}
	node, err := s.uc.CompleteUpload(ctx, uploadID, req.GetEtag(), req.GetComment())
	if err != nil {
		return nil, err
	}
	return s.renderNode(ctx, node)
}

// AbortUpload cancels a session and reclaims its staged parts.
func (s *FileService) AbortUpload(ctx context.Context, req *v1.AbortUploadRequest) (*emptypb.Empty, error) {
	uploadID, err := parseUUID(req.GetUploadId())
	if err != nil {
		return nil, err
	}
	if err := s.uc.AbortUpload(ctx, uploadID); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// ConfirmUploadPart records a part uploaded straight to object storage.
func (s *FileService) ConfirmUploadPart(ctx context.Context, req *v1.ConfirmUploadPartRequest) (*v1.UploadPart, error) {
	uploadID, err := parseUUID(req.GetUploadId())
	if err != nil {
		return nil, err
	}
	part, _, err := s.uc.ConfirmUploadPart(ctx, uploadID, req.GetPartNumber(), req.GetEtag())
	if err != nil {
		return nil, err
	}
	return convertUploadPart(part), nil
}

// UploadSmallFile stores a payload carried inside the request.
func (s *FileService) UploadSmallFile(ctx context.Context, req *v1.UploadSmallFileRequest) (*v1.Node, error) {
	parentID, err := parseOptionalUUID(req.GetParentId())
	if err != nil {
		return nil, err
	}
	node, err := s.uc.UploadSmallFile(ctx, biz.UploadSmallFileInput{
		ParentID:    parentID,
		Name:        req.GetName(),
		Content:     req.GetContent(),
		MimeType:    req.GetMimeType(),
		Policy:      parseConflictPolicy(req.GetConflictPolicy()),
		Description: req.GetDescription(),
		Metadata:    req.GetMetadata(),
	})
	if err != nil {
		return nil, err
	}
	return s.renderNode(ctx, node)
}

// GetUpload returns one upload session.
func (s *FileService) GetUpload(ctx context.Context, req *v1.GetUploadRequest) (*v1.UploadSession, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, err
	}
	upload, parts, err := s.uc.GetUpload(ctx, id)
	if err != nil {
		return nil, err
	}
	return convertUpload(upload, parts), nil
}

// ListUploadParts returns the parts already stored for a session.
func (s *FileService) ListUploadParts(ctx context.Context, req *v1.ListUploadPartsRequest) (*v1.UploadPartSet, error) {
	uploadID, err := parseUUID(req.GetUploadId())
	if err != nil {
		return nil, err
	}
	parts, _, err := s.uc.ListUploadParts(ctx, uploadID, req.GetFromPartNumber())
	if err != nil {
		return nil, err
	}
	set := &v1.UploadPartSet{UploadId: uploadID.String(), Parts: make([]*v1.UploadPart, 0, len(parts))}
	for _, p := range parts {
		set.Parts = append(set.Parts, convertUploadPart(p))
	}
	return set, nil
}

// UploadChunk streams one part through the server. Over JSON the payload is a
// base64 string; the raw streaming binding sends the same bytes as an
// application/octet-stream body.
func (s *FileService) UploadChunk(ctx context.Context, req *v1.UploadChunkRequest) (*v1.UploadPart, error) {
	uploadID, err := parseUUID(req.GetUploadId())
	if err != nil {
		return nil, err
	}
	if req.GetPartNumber() < 1 {
		return nil, biz.ErrInvalidArgument
	}
	content := req.GetContent()
	part, _, err := s.uc.UploadChunk(ctx, biz.UploadChunkInput{
		UploadID:   uploadID,
		PartNumber: req.GetPartNumber(),
		Size:       int64(len(content)),
		Reader:     bytesReader(content),
		Etag:       req.GetEtag(),
	})
	if err != nil {
		return nil, err
	}
	return convertUploadPart(part), nil
}

// GetDownloadUrl returns a signed URL for the original bytes of a file.
func (s *FileService) GetDownloadUrl(ctx context.Context, req *v1.GetDownloadUrlRequest) (*v1.SignedUrl, error) {
	nodeID, err := parseUUID(req.GetNodeId())
	if err != nil {
		return nil, err
	}
	signed, err := s.uc.DownloadURL(ctx, nodeID, req.GetExpiresInSeconds(), req.GetInline(), req.GetFileName())
	if err != nil {
		return nil, err
	}
	return convertSignedURL(signed), nil
}

// GetArchiveUrl returns a signed URL that streams a folder as a zip archive.
func (s *FileService) GetArchiveUrl(ctx context.Context, req *v1.GetArchiveUrlRequest) (*v1.SignedUrl, error) {
	nodeID, err := parseUUID(req.GetNodeId())
	if err != nil {
		return nil, err
	}
	signed, err := s.uc.ArchiveURL(ctx, nodeID, req.GetExpiresInSeconds(), req.GetArchiveName())
	if err != nil {
		return nil, err
	}
	return convertSignedURL(signed), nil
}

// GetPreviewUrl returns a signed URL for an inline preview.
func (s *FileService) GetPreviewUrl(ctx context.Context, req *v1.GetPreviewUrlRequest) (*v1.SignedUrl, error) {
	nodeID, err := parseUUID(req.GetNodeId())
	if err != nil {
		return nil, err
	}
	signed, err := s.uc.PreviewURL(ctx, nodeID, req.GetExpiresInSeconds())
	if err != nil {
		return nil, err
	}
	return convertSignedURL(signed), nil
}

// renderNode decorates a node the usecase just returned.
func (s *FileService) renderNode(ctx context.Context, node *biz.Node) (*v1.Node, error) {
	access, err := s.nodes.Evaluate(ctx, biz.CallerFromContext(ctx), node)
	if err != nil {
		return nil, err
	}
	display, err := s.nodes.DisplayPath(ctx, node)
	if err != nil {
		display = ""
	}
	owners, _ := s.users.Names(ctx, []uuid.UUID{node.OwnerID})
	return convertNodeOwned(node, access, display, owners[node.OwnerID], 0), nil
}
