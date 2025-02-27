package frostdb

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"github.com/segmentio/parquet-go"
	"github.com/thanos-io/objstore"
	"go.opentelemetry.io/otel/attribute"

	"github.com/polarsignals/frostdb/dynparquet"
)

// Persist uploads the block to the underlying bucket.
func (t *TableBlock) Persist() error {
	if t.table.db.bucket == nil {
		return nil
	}

	r, w := io.Pipe()
	var err error
	go func() {
		defer w.Close()
		err = t.Serialize(w)
	}()
	defer r.Close()

	fileName := filepath.Join(t.table.name, t.ulid.String(), "data.parquet")
	if err := t.table.db.bucket.Upload(context.Background(), fileName, r); err != nil {
		return fmt.Errorf("failed to upload block %v", err)
	}

	if err != nil {
		return fmt.Errorf("failed to serialize block: %v", err)
	}
	return nil
}

func (t *Table) IterateBucketBlocks(ctx context.Context, logger log.Logger, filter TrueNegativeFilter, iterator func(rg dynparquet.DynamicRowGroup) bool, lastBlockTimestamp uint64) error {
	ctx, span := t.tracer.Start(ctx, "Table/IterateBucketBlocks")
	span.SetAttributes(attribute.Int64("lastBlockTimestamp", int64(lastBlockTimestamp)))
	defer span.End()

	if t.db.bucket == nil || t.db.ignoreStorageOnQuery {
		return nil
	}

	n := 0
	err := t.db.bucket.Iter(ctx, t.name, func(blockDir string) error {
		ctx, span := t.tracer.Start(ctx, "Table/IterateBucketBlocks/Iter")
		defer span.End()

		blockUlid, err := ulid.Parse(filepath.Base(blockDir))
		if err != nil {
			return err
		}

		span.SetAttributes(attribute.String("ulid", blockUlid.String()))

		if lastBlockTimestamp != 0 && blockUlid.Time() >= lastBlockTimestamp {
			return nil
		}

		blockName := filepath.Join(blockDir, "data.parquet")
		attribs, err := t.db.bucket.Attributes(ctx, blockName)
		if err != nil {
			return err
		}

		span.SetAttributes(attribute.Int64("size", attribs.Size))

		b := &BucketReaderAt{
			name:   blockName,
			ctx:    ctx,
			Bucket: t.db.bucket,
		}

		file, err := parquet.OpenFile(b, attribs.Size)
		if err != nil {
			return err
		}

		// Get a reader from the file bytes
		buf, err := dynparquet.NewSerializedBuffer(file)
		if err != nil {
			return err
		}

		n++
		for i := 0; i < buf.NumRowGroups(); i++ {
			span.AddEvent("rowgroup")

			rg := buf.DynamicRowGroup(i)
			var mayContainUsefulData bool
			mayContainUsefulData, err = filter.Eval(rg)
			if err != nil {
				return err
			}
			if mayContainUsefulData {
				if continu := iterator(rg); !continu {
					return err
				}
			}
		}
		return nil
	})
	level.Debug(logger).Log("msg", "read blocks", "n", n)
	return err
}

// BucketReaderAt is an objstore.Bucket wrapper that supports the io.ReaderAt interface.
type BucketReaderAt struct {
	name string
	ctx  context.Context
	objstore.Bucket
}

// ReadAt implements the io.ReaderAt interface.
func (b *BucketReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	rc, err := b.GetRange(b.ctx, b.name, off, int64(len(p)))
	if err != nil {
		return 0, err
	}
	defer func() {
		err = rc.Close()
	}()

	return rc.Read(p)
}
