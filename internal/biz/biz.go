package biz

import "github.com/google/wire"

// ProviderSet is biz providers.
var ProviderSet = wire.NewSet(
	NewUserUsecase,
	NewNodeUsecase,
	NewFileUsecase,
	NewShareUsecase,
	NewAuthUsecase,
	NewAuditUsecase,
	NewSystemUsecase,
)
