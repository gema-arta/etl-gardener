// +build integration

package bq_test

import (
	"context"
	"log"
	"testing"

	"github.com/m-lab/etl-gardener/cloud/bq"
	"github.com/m-lab/go/dataset"
)

func init() {
	// Always prepend the filename and line number.
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func TestGetTableDetail(t *testing.T) {
	ctx := context.Background()
	destDS, err := dataset.NewDataset(ctx, "mlab-testing", "etl")
	if err != nil {
		t.Fatal(err)
	}

	// Check that it handles empty partitions
	detail, err := bq.GetTableDetail(ctx, &destDS, destDS.Table("DedupTest$20001229"))
	if err != nil {
		t.Error(err)
	} else if detail.TaskFileCount > 0 || detail.TestCount > 0 {
		t.Error("Should have zero counts")
	}

	// Check that it handles single partitions.
	// TODO - update to create its own test table.
	detail, err = bq.GetTableDetail(ctx, &destDS, destDS.Table("DedupTest$19990101"))
	if err != nil {
		t.Error(err)
	} else if detail.TaskFileCount != 2 || detail.TestCount != 4 {
		t.Error("Wrong number of tasks or tests")
	}

	srcDS, err := dataset.NewDataset(ctx, "mlab-testing", "src")
	if err != nil {
		t.Fatal(err)
	}

	// Check that it handles full table.
	// TODO - update to create its own test table.
	detail, err = bq.GetTableDetail(ctx, &srcDS, srcDS.Table("DedupTest_19990101"))
	if err != nil {
		t.Error(err)
	} else if detail.TaskFileCount != 2 || detail.TestCount != 6 {
		t.Error("Wrong number of tasks or tests")
	}
}

func TestAnnotationTableMeta(t *testing.T) {
	// TODO - Make NewDataSet return a pointer, for consistency with bigquery.
	ctx := context.Background()
	dsExt, err := dataset.NewDataset(ctx, "mlab-testing", "src")
	if err != nil {
		t.Fatal(err)
	}

	tbl := dsExt.Table("DedupTest")
	at := bq.NewAnnotatedTable(tbl, &dsExt)
	meta, err := at.CachedMeta(ctx)
	if err != nil {
		t.Error(err)
	} else if meta.NumRows != 8 {
		t.Errorf("Wrong number of rows: %d", meta.NumRows)
	}
	if meta.TimePartitioning == nil {
		t.Error("Should be partitioned")
	}

	tbl = dsExt.Table("XYZ")
	at = bq.NewAnnotatedTable(tbl, &dsExt)
	meta, err = at.CachedMeta(nil)
	if err != bq.ErrNilContext {
		t.Error("Should be an error when no context provided")
	}
	meta, err = at.CachedMeta(ctx)
	if err == nil {
		t.Error("Should be an error when fetching bad table meta")
	}
}

func TestAnnotationDetail(t *testing.T) {
	ctx := context.Background()
	dsExt, err := dataset.NewDataset(ctx, "mlab-testing", "src")
	if err != nil {
		t.Fatal(err)
	}

	tbl := dsExt.Table("DedupTest")
	meta, err := tbl.Metadata(ctx)
	if err != nil {
		t.Error(err)
	} else if meta == nil {
		t.Error("Meta should not be nil")
	}
	at := bq.NewAnnotatedTable(tbl, &dsExt)
	_, err = at.CachedDetail(ctx)
	if err != nil {
		t.Error(err)
	}
}

func TestGetTablesMatching(t *testing.T) {
	ctx := context.Background()
	dsExt, err := dataset.NewDataset(ctx, "mlab-testing", "src")
	if err != nil {
		t.Fatal(err)
	}

	atList, err := bq.GetTablesMatching(ctx, &dsExt, "Test")
	if err != nil {
		t.Error(err)
	} else if len(atList) != 3 {
		t.Errorf("Wrong length: %d", len(atList))
	}
}

func TestAnnotatedTableGetPartitionInfo(t *testing.T) {
	ctx := context.Background()
	dsExt, err := dataset.NewDataset(ctx, "mlab-testing", "src")
	if err != nil {
		t.Fatal(err)
	}

	tbl := dsExt.Table("DedupTest$19990101")
	at := bq.NewAnnotatedTable(tbl, &dsExt)
	info, err := at.GetPartitionInfo(ctx)
	if err != nil {
		t.Error(err)
	} else if info.PartitionID != "19990101" {
		t.Error("wrong partitionID: " + info.PartitionID)
	}

	// Check behavior for missing partition
	tbl = dsExt.Table("DedupTest$17760101")
	at = bq.NewAnnotatedTable(tbl, &dsExt)
	info, err = at.GetPartitionInfo(ctx)
	if err != nil {
		t.Error(err)
	} else if info.PartitionID != "" {
		t.Error("Non-existent partition should return empty PartitionID")
	}
}