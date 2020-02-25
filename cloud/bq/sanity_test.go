package bq

import (
	"context"
	"log"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"

	"github.com/googleapis/google-cloud-go-testing/bigquery/bqiface"
	"github.com/m-lab/etl-gardener/cloud"
	"github.com/m-lab/go/dataset"
	"google.golang.org/api/option"
)

func init() {
	// Always prepend the filename and line number.
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

type testDataset struct {
	bqiface.Dataset
}

func (ds *testDataset) Table(name string) bqiface.Table {
	tt := testTable{ds.Dataset.Table(name)}
	return tt
}

// creates a Dataset with a dry run client.
func newTestDataset(project, ds string) dataset.Dataset {
	ctx := context.Background()
	dryRun, _ := cloud.DryRunClient()
	c, err := bigquery.NewClient(ctx, project, option.WithHTTPClient(dryRun))
	if err != nil {
		panic(err)
	}

	bqClient := bqiface.AdaptClient(c)

	return dataset.Dataset{Dataset: &testDataset{bqClient.Dataset(ds)}, BqClient: bqClient}
}

// This defines a Dataset that returns a Table, that returns a canned Metadata.
type testTable struct {
	bqiface.Table
}

func (tbl testTable) Metadata(ctx context.Context) (*bigquery.TableMetadata, error) {
	meta := bigquery.TableMetadata{CreationTime: time.Now(), LastModifiedTime: time.Now(), NumBytes: 168, NumRows: 8}
	meta.TimePartitioning = &bigquery.TimePartitioning{Expiration: 0 * time.Second}
	return &meta, nil
}

func TestSanityCheckAndCopyTesting(t *testing.T) {
	ctx := context.Background()
	ds, err := dataset.NewDataset(ctx, "mlab-staging", "batch")
	if err != nil {
		t.Fatal(err)
	}
	src := ds.Table("traceroute_20130524")
	dest := ds.Table("traceroute$20130524")
	srcAt := NewAnnotatedTable(src, &ds)
	destAt := NewAnnotatedTable(dest, &ds)

	err = SanityCheckAndCopy(ctx, srcAt, destAt)
	log.Println(err)
}

// getTableParts separates a table name into prefix/base, separator, and partition date.
func Test_getTableParts(t *testing.T) {
	parts, err := getTableParts("table$20160102")
	if err != nil {
		t.Error(err)
	} else {
		if !parts.isPartitioned {
			t.Error("Should be partitioned")
		}
		if parts.prefix != "table" {
			t.Error("incorrect prefix: " + parts.prefix)
		}
		if parts.yyyymmdd != "20160102" {
			t.Error("incorrect partition: " + parts.yyyymmdd)
		}
	}

	parts, err = getTableParts("table_20160102")
	if err != nil {
		t.Error(err)
	} else {
		if parts.isPartitioned {
			t.Error("Should not be partitioned")
		}
		if parts.prefix != "table" {
			t.Error("incorrect prefix: " + parts.prefix)
		}
		if parts.yyyymmdd != "20160102" {
			t.Error("incorrect partition: " + parts.yyyymmdd)
		}
	}
	parts, err = getTableParts("table$2016010")
	if err == nil {
		t.Error("Should error when partition is incomplete")
	}
	parts, err = getTableParts("table$201601022")
	if err == nil {
		t.Error("Should error when partition is too long")
	}
	parts, err = getTableParts("table$20162102")
	if err == nil {
		t.Error("Should error when partition is invalid")
	}
}

/*
func TestSanityCheckAndCopy(t *testing.T) {
	ctx := context.Background()
	ds, err := dataset.NewDataset(ctx, "project", "dataset")
	if err != nil {
		t.Fatal(err)
	}
	src := ds.Table("foo_19990101")
	dest := ds.Table("foo$19990101")
	srcAt := NewAnnotatedTable(src, &ds)
	destAt := NewAnnotatedTable(dest, &ds)

	err = SanityCheckAndCopy(ctx, srcAt, destAt)
	if err == nil {
		t.Fatal("Should have 404 error")
	}
	if !strings.HasPrefix(err.Error(), "googleapi: Error 404") {
		t.Fatal(err)
	}
}

func TestCachedMeta(t *testing.T) {
	ctx := context.Background()
	dsExt := newTestDataset("mlab-testing", "etl")

	tbl := dsExt.Table("DedupTest")
	meta, err := tbl.Metadata(ctx)
	if err != nil {
		t.Error(err)
	} else if meta == nil {
		t.Error("Meta should not be nil")
	}

	at := NewAnnotatedTable(tbl, &dsExt)
	// Fetch cache detail - which hits backend
	meta, err = at.CachedMeta(ctx)
	if err != nil {
		t.Error(err)
	} else if meta == nil {
		t.Error("Meta should not be nil")
	}
	// Fetch again, exercising the cached code path.
	meta, err = at.CachedMeta(ctx)
	if err != nil {
		t.Error(err)
	} else if meta == nil {
		t.Error("Meta should not be nil")
	}

}
*/
