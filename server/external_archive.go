package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func syncXRWArchive(ctx context.Context, db *pgxpool.Pool, filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	snapshot, err := readXRWArchive(ctx, file)
	if err != nil {
		return err
	}
	return writeXRWAlbums(ctx, db, snapshot.albums, snapshot.builtAt)
}

type xrwSnapshot struct {
	builtAt time.Time
	albums  []xrwAlbum
}

func readXRWArchive(ctx context.Context, file io.Reader) (xrwSnapshot, error) {
	var snapshot xrwSnapshot
	var manifest struct {
		BuiltAt    time.Time `json:"builtAt"`
		AlbumCount int       `json:"albumCount"`
		PhotoCount int       `json:"photoCount"`
	}
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return snapshot, err
	}
	defer compressed.Close()
	archive := tar.NewReader(compressed)
	seen := map[string]bool{}
	photos := 0
	for {
		if err := ctx.Err(); err != nil {
			return snapshot, err
		}
		entry, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return snapshot, err
		}
		_, name, _ := strings.Cut(entry.Name, "/")
		if entry.Typeflag != tar.TypeReg {
			continue
		}
		if name == "data/manifest.json" {
			if err := json.NewDecoder(io.LimitReader(archive, 16<<10)).Decode(&manifest); err != nil {
				return snapshot, err
			}
			continue
		}
		if !strings.HasPrefix(name, "data/photos/") || !strings.HasSuffix(name, ".json") {
			continue
		}
		albumID := strings.TrimSuffix(strings.TrimPrefix(name, "data/photos/"), ".json")
		if !xrwAlbumID.MatchString(albumID) || seen[albumID] {
			return snapshot, fmt.Errorf("invalid or duplicate album index %q", albumID)
		}
		album, err := decodeXRWAlbum(archive, albumID)
		if err != nil {
			return snapshot, fmt.Errorf("xrw-album %s: %w", albumID, err)
		}
		seen[albumID] = true
		photos += len(album.Photos)
		snapshot.albums = append(snapshot.albums, album)
	}
	if _, err := io.Copy(io.Discard, compressed); err != nil {
		return snapshot, err
	}
	if manifest.BuiltAt.IsZero() || len(snapshot.albums) == 0 || len(snapshot.albums) != manifest.AlbumCount || photos != manifest.PhotoCount {
		return snapshot, fmt.Errorf("xrw-album manifest mismatch: read %d albums and %d photos", len(snapshot.albums), photos)
	}
	snapshot.builtAt = manifest.BuiltAt
	return snapshot, nil
}
