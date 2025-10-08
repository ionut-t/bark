package assets

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
)

// Asset holds the name and prompt for a given asset.
type Asset struct {
	Name   string
	Prompt string
}

type ConfigOptions struct {
	Storage  string
	Reset    bool
	AssetDir string
	FromDir  string
	EmbedFS  embed.FS
}

// ConfigAssets unpacks the embedded assets into the storage directory.
func ConfigAssets(opts ConfigOptions) error {
	assetsDir := filepath.Join(opts.Storage, opts.AssetDir)

	fs := opts.EmbedFS

	if _, err := os.Stat(assetsDir); err == nil {
		if !opts.Reset {
			return nil
		}
	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(assetsDir, 0755); err != nil {
			return err
		}
	}

	entries, err := fs.ReadDir(opts.FromDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			filename := filepath.Join(opts.FromDir, entry.Name())
			content, err := fs.ReadFile(filename)
			if err != nil {
				return err
			}

			path := filepath.Join(opts.Storage, opts.AssetDir, entry.Name())

			err = os.WriteFile(path, content, 0644)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// GetAssets loads all assets from the storage directory.
func GetAssets(storage string, assetDirName string) ([]Asset, error) {
	var assetsList []Asset

	assetsDir := filepath.Join(storage, assetDirName)
	entries, err := os.ReadDir(assetsDir)
	if err != nil {
		return assetsList, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			content, err := os.ReadFile(filepath.Join(assetsDir, entry.Name()))
			if err != nil {
				return assetsList, err
			}

			name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))

			asset := Asset{
				Name:   name,
				Prompt: string(content),
			}
			assetsList = append(assetsList, asset)
		}
	}

	return assetsList, nil
}

// RemoveAssetDir removes the asset directory from storage.
func RemoveAssetDir(storage string, assetDirName string) error {
	return os.RemoveAll(filepath.Join(storage, assetDirName))
}
