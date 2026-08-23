package facade

import "context"

type FileAssetBindingFacade interface {
	BindUploadedFile(ctx context.Context, command BindUploadedFileCommand) (*FileReferenceDTO, error)
}

// ConfigAssetFacade owns the finite CONFIG_ASSET lifecycle. Keeping this
// separate from FileAssetBindingFacade prevents unrelated business modules
// from turning a generic fileId into an arbitrary configuration reference.
type ConfigAssetFacade interface {
	BindConfigAsset(ctx context.Context, command BindConfigAssetCommand) error
	UpdateConfigAssetPolicy(ctx context.Context, command UpdateConfigAssetPolicyCommand) error
	ClearConfigAsset(ctx context.Context, configID int64) error
	CaptureConfigAssetBinding(ctx context.Context, command CaptureConfigAssetBindingCommand) (ConfigAssetBindingState, error)
	RestoreConfigAssetBinding(ctx context.Context, command RestoreConfigAssetBindingCommand) error
	OpenConfigAsset(ctx context.Context, configID int64) (*ConfigAssetOpenResult, error)
}

type Facades struct {
	Assets       FileAssetBindingFacade
	ConfigAssets ConfigAssetFacade
}
