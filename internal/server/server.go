package server

import (
	"nagisa/internal/biz"
	"nagisa/internal/server/middleware"

	"github.com/google/wire"
)

// ProviderSet is server providers.
var ProviderSet = wire.NewSet(NewGRPCServer, NewHTTPServer, NewStreamHandler, NewAuditor)

// NewAuditor starts the background audit writer and returns its shutdown hook.
func NewAuditor(repo biz.AuditRepo, tx biz.TxManager) (*middleware.Auditor, func()) {
	auditor := middleware.NewAuditor(repo, tx)
	return auditor, auditor.Close
}

var _ = wire.Bind
