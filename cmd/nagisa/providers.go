package main

import (
	"time"

	"nagisa/internal/biz"
	"nagisa/internal/conf"
	"nagisa/internal/pkg/crypt"
	"nagisa/internal/pkg/urlsign"

	"github.com/go-kratos/kratos/v3/log"
	"github.com/google/wire"
)

// providerSet turns the bootstrap configuration into the value types the
// domain layer expects, and binds the concrete infrastructure types onto the
// domain interfaces. It lives in the command because wiring is the only place
// that is allowed to know every layer at once.
var providerSet = wire.NewSet(
	provideServerConfig,
	provideDataConfig,
	provideWebConfig,
	provideHasher,
	provideKeyBox,
	provideIssuer,
	provideURLSigner,
	provideUserOptions,
	provideNodeOptions,
	provideUploadLimits,
	provideShareOptions,
	provideAuthOptions,
	provideSystemOptions,
	wire.Bind(new(biz.Hasher), new(*crypt.Hasher)),
	wire.Bind(new(biz.PasswordVerifier), new(*crypt.Hasher)),
)

func provideServerConfig(c *conf.Bootstrap) *conf.Server { return c.GetServer() }

func provideDataConfig(c *conf.Bootstrap) *conf.Data { return c.GetData() }

func provideWebConfig(c *conf.Bootstrap) *conf.Web { return c.GetWeb() }

func provideHasher(c *conf.Bootstrap) *crypt.Hasher {
	return crypt.NewHasher(crypt.DefaultBcryptCost)
}

// provideKeyBox loads the RSA key used to decode password fields, generating
// and persisting one on first start so a restart keeps accepting in-flight
// logins.
func provideKeyBox(c *conf.Bootstrap) (*crypt.KeyBox, error) {
	a := c.GetAuth()
	path := a.GetPasswordPrivateKeyFile()
	if path == "" && a.GetPasswordPrivateKey() == "" {
		path = "./data/password_key.pem"
	}
	return crypt.LoadOrCreateKeyBox(a.GetPasswordPrivateKey(), path)
}

// provideIssuer builds the token signer. A missing secret is generated for the
// current process, which invalidates tokens on restart; that is logged loudly
// because production must configure it.
func provideIssuer(c *conf.Bootstrap) *crypt.Issuer {
	secret := c.GetAuth().GetJwtSecret()
	if secret == "" {
		generated, err := crypt.RandomToken(32)
		if err != nil {
			panic(err)
		}
		secret = generated
		log.Warn("auth.jwt_secret is unset, a random secret was generated and every token will be invalid after a restart")
	}
	issuer := c.GetAuth().GetIssuer()
	if issuer == "" {
		issuer = "nagisa-netdisk"
	}
	return crypt.NewIssuer(secret, issuer)
}

// provideURLSigner signs the streaming URLs the server serves itself.
func provideURLSigner(c *conf.Bootstrap) biz.URLSigner {
	secret := c.GetAuth().GetJwtSecret()
	if secret == "" {
		generated, err := crypt.RandomToken(32)
		if err != nil {
			panic(err)
		}
		secret = generated
	}
	return urlsign.New(c.GetWeb().GetPublicBaseUrl(), secret)
}

func provideUserOptions(c *conf.Bootstrap) biz.UserUsecaseOptions {
	return biz.UserUsecaseOptions{
		DefaultQuotaBytes: c.GetStorage().GetDefaultQuotaBytes(),
		DefaultRolePreset: c.GetStorage().GetDefaultRolePreset(),
		AdminUsername:     c.GetAuth().GetAdminUsername(),
		GuestUsername:     c.GetAuth().GetGuestUsername(),
	}
}

func provideNodeOptions(c *conf.Bootstrap) biz.NodeUsecaseOptions {
	s := c.GetStorage()
	return biz.NodeUsecaseOptions{
		MaxDepth:             s.GetMaxPathDepth(),
		MaxNameLength:        int(s.GetMaxNameLength()),
		CaseInsensitiveNames: s.GetCaseInsensitiveNames(),
		RootName:             s.GetRootFolderName(),
		DefaultVisibility:    visibility(s.GetDefaultVisibility()),
	}
}

func provideUploadLimits(c *conf.Bootstrap) biz.UploadLimits {
	u := c.GetUpload()
	return biz.UploadLimits{
		DefaultChunkSize: u.GetDefaultChunkSize(),
		MinChunkSize:     u.GetMinChunkSize(),
		MaxChunkSize:     u.GetMaxChunkSize(),
		MaxFileSize:      u.GetMaxFileSize(),
		MaxInlineSize:    u.GetMaxInlineSize(),
		MaxParts:         u.GetMaxParts(),
		SessionTTL:       u.GetSessionTtl().AsDuration(),
		PresignTTL:       c.GetData().GetObjectStorage().GetPresignTtl().AsDuration(),
		DefaultMode:      uploadMode(u.GetDefaultMode()),
		VerifyChecksum:   u.GetVerifyChecksum(),
		KeepVersions:     u.GetKeepVersions(),
		MaxVersions:      u.GetMaxVersions(),
	}
}

func provideShareOptions(c *conf.Bootstrap) biz.ShareUsecaseOptions {
	return biz.ShareUsecaseOptions{
		AllowPublic:     c.GetStorage().GetAllowPublicShare(),
		PublicBaseURL:   c.GetWeb().GetPublicBaseUrl(),
		SharePathPrefix: c.GetWeb().GetSharePathPrefix(),
		AccessTokenTTL:  2 * time.Hour,
	}
}

func provideAuthOptions(c *conf.Bootstrap) biz.AuthUsecaseOptions {
	a := c.GetAuth()
	return biz.AuthUsecaseOptions{
		AccessTokenTTL:     a.GetAccessTokenTtl().AsDuration(),
		RefreshTokenTTL:    a.GetRefreshTokenTtl().AsDuration(),
		NodeTokenTTL:       a.GetNodeTokenTtl().AsDuration(),
		PlainPasswordAllow: a.GetAllowPlainPassword(),
		MinPasswordLength:  int(a.GetMinPasswordLength()),
		GuestUsername:      a.GetGuestUsername(),
		GuestAutoLogin:     a.GetGuestAutoLogin(),
	}
}

// provideSystemOptions describes the deployment. The upload limits come from
// the file usecase rather than straight from the configuration, so the report
// carries the values actually in force once its defaults were applied: a
// deployment that leaves the upload section out still advertises the real
// chunk sizes and session lifetime instead of zeroes.
func provideSystemOptions(c *conf.Bootstrap, files *biz.FileUsecase) biz.SystemUsecaseOptions {
	u := c.GetUpload()
	s := c.GetStorage()
	limits := files.Limits()
	backend := "none"
	if c.GetData().GetObjectStorage().GetEndpoint() != "" {
		backend = "s3"
	}
	database := c.GetData().GetDatabase().GetDriver()
	if database == "" {
		database = "sqlite"
	}
	modes := []biz.UploadMode{biz.UploadModePresigned, biz.UploadModeProxy}
	switch uploadMode(u.GetDefaultMode()) {
	case biz.UploadModeProxy:
		modes = []biz.UploadMode{biz.UploadModeProxy, biz.UploadModePresigned}
	}
	return biz.SystemUsecaseOptions{
		TrashRetention: s.GetTrashRetention().AsDuration(),
		Info: biz.SystemInfo{
			Name:              Name,
			Version:           Version,
			APIVersion:        "v1",
			Features:          systemFeatures(c, backend),
			MaxUploadSize:     limits.MaxFileSize,
			DefaultChunkSize:  limits.DefaultChunkSize,
			MinChunkSize:      limits.MinChunkSize,
			MaxInlineSize:     limits.MaxInlineSize,
			UploadSessionTTL:  limits.SessionTTL,
			SignedURLTTL:      limits.PresignTTL,
			SignedURLMaxTTL:   4 * limits.PresignTTL,
			UploadModes:       modes,
			StorageBackend:    backend,
			DatabaseBackend:   database,
			DefaultVisibility: visibility(s.GetDefaultVisibility()),
			RegistrationOpen:  false,
			PublicBaseURL:     c.GetWeb().GetPublicBaseUrl(),
		},
	}
}

// systemFeatures lists the capabilities this deployment actually offers.
//
// The list follows the configuration instead of advertising everything the
// binary can do: a client uses it to decide whether to offer a feature, and a
// feature that is switched off answers NETDISK_UNSUPPORTED. Object storage is
// the exception that stays truthful through `storage_backend: "none"`.
func systemFeatures(c *conf.Bootstrap, backend string) []string {
	features := []string{
		"chunked_upload", "presigned_upload", "proxy_upload", "signed_download",
		"share_links", "node_password", "acl", "versions",
		"trash", "audit", "quota", "folder_archive", "range_download",
	}
	if c.GetStorage().GetAllowPublicShare() {
		features = append(features, "public_share")
	}
	if backend != "none" {
		return features
	}
	// Without object storage nothing can be transferred, so the transfer
	// capabilities are not advertised either.
	transfer := map[string]bool{
		"chunked_upload": true, "presigned_upload": true, "proxy_upload": true,
		"signed_download": true, "folder_archive": true, "range_download": true,
	}
	kept := make([]string, 0, len(features))
	for _, feature := range features {
		if !transfer[feature] {
			kept = append(kept, feature)
		}
	}
	return kept
}

// uploadMode resolves the configured default transport.
func uploadMode(name string) biz.UploadMode {
	if mode, ok := biz.ParseUploadMode(name); ok {
		return mode
	}
	return biz.UploadModePresigned
}

// visibility resolves the configured default visibility.
func visibility(name string) biz.Visibility {
	if v, ok := biz.ParseVisibility(name); ok {
		return v
	}
	return biz.VisibilityPrivate
}
