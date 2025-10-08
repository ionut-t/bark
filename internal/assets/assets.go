package assets

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ionut-t/bark/internal/utils"
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

func Add(storage string, assetDirName string, name string) error {
	assetsDir := filepath.Join(storage, assetDirName)

	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		return fmt.Errorf("error creating assets directory: %w", err)
	}

	finalPath := filepath.Join(assetsDir, name+".md")

	if _, err := os.Stat(finalPath); err == nil {
		return os.ErrExist
	}

	tmpFile, err := os.CreateTemp("", name+"-*.md")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	if err := utils.OpenEditor(tmpPath); err != nil {
		return fmt.Errorf("error opening editor: %w", err)
	}

	content, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("error reading temporary file: %w", err)
	}

	if len(content) == 0 {
		return errors.New("content cannot be empty")
	}

	if err := os.WriteFile(finalPath, content, 0644); err != nil {
		return err
	}

	return nil
}
