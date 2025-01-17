package compiler

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"

	cueload "cuelang.org/go/cue/load"
	"go.opentelemetry.io/otel"
)

// Build a cue configuration tree from the files in fs.
func Build(ctx context.Context, src string, overlays map[string]fs.FS, args ...string) (*Value, error) {
	_, span := otel.Tracer("dagger").Start(ctx, "compiler.Build")
	defer span.End()

	c := DefaultCompiler

	buildConfig := &cueload.Config{
		Dir:     src,
		Overlay: map[string]cueload.Source{},
	}

	// Map the source files into the overlay
	for mnt, f := range overlays {
		f := f
		mnt := mnt
		err := fs.WalkDir(f, ".", func(p string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if !entry.Type().IsRegular() {
				return nil
			}

			if filepath.Ext(entry.Name()) != ".cue" {
				return nil
			}

			contents, err := fs.ReadFile(f, p)
			if err != nil {
				return fmt.Errorf("%s: %w", p, err)
			}

			overlayPath := path.Join(buildConfig.Dir, mnt, p)
			buildConfig.Overlay[overlayPath] = cueload.FromBytes(contents)
			return nil
		})
		if err != nil {
			return nil, Err(err)
		}
	}
	instances := cueload.Instances(args, buildConfig)
	if len(instances) != 1 {
		return nil, errors.New("only one package is supported at a time")
	}
	for _, value := range instances {
		if value.Err != nil {
			return nil, Err(value.Err)
		}
	}
	v, err := c.Context.BuildInstances(instances)
	if err != nil {
		return nil, c.Err(err)
	}
	for _, value := range v {
		if value.Err() != nil {
			return nil, c.Err(value.Err())
		}
	}
	if len(v) != 1 {
		return nil, errors.New("internal: wrong number of values")
	}
	return Wrap(v[0]), nil
}
