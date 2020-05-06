package bq

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"

	"cloud.google.com/go/bigquery"
	"github.com/googleapis/google-cloud-go-testing/bigquery/bqiface"

	"github.com/m-lab/go/dataset"

	"github.com/m-lab/etl-gardener/tracker"
)

// OpsHandler provides the interface for running bigquery operations.
type OpsHandler interface {
	DedupQuery() string

	Dedup(ctx context.Context, dryRun bool) (bqiface.Job, error)
	CopyToRaw(ctx context.Context, dryRun bool) (bqiface.Job, error)
	DeleteTmp(ctx context.Context) error
}

// queryer is used to construct a dedup query.
type queryer struct {
	client  bqiface.Client
	Project string
	Date    string // Name of the partition field
	Job     tracker.Job
	// map key is the single field name, value is fully qualified name
	PartitionKeys map[string]string
	OrderKeys     string
}

// ErrDatatypeNotSupported is returned by Query for unsupported datatypes.
var ErrDatatypeNotSupported = errors.New("Datatype not supported")

// NewQuerier creates a suitable QueryParams for a Job.
func NewQuerier(job tracker.Job, project string) (OpsHandler, error) {
	c, err := bigquery.NewClient(context.Background(), project)
	if err != nil {
		return nil, err
	}
	bqClient := bqiface.AdaptClient(c)
	return NewQuerierWithClient(bqClient, job, project)
}

// NewQuerierWithClient creates a suitable QueryParams for a Job.
func NewQuerierWithClient(client bqiface.Client, job tracker.Job, project string) (OpsHandler, error) {
	switch job.Datatype {
	case "annotation":
		return &queryer{
			client:        client,
			Project:       project,
			Date:          "date",
			Job:           job,
			PartitionKeys: map[string]string{"id": "id"},
			OrderKeys:     "",
		}, nil

	case "ndt5":
		return &queryer{
			client:        client,
			Project:       project,
			Date:          "log_time",
			Job:           job,
			PartitionKeys: map[string]string{"test_id": "test_id"},
			OrderKeys:     "",
		}, nil

	case "ndt7":
		return &queryer{
			client:        client,
			Project:       project,
			Date:          "date",
			Job:           job,
			PartitionKeys: map[string]string{"id": "id"},
			OrderKeys:     "",
		}, nil

//	case "tcpinfo":
//		return &queryer{
//			client:        client,
//			Project:       project,
//			Date:          "DATE(TestTime)",
//			Job:           job,
//			PartitionKeys: map[string]string{"uuid": "uuid", "Timestamp": "FinalSnapshot.Timestamp"},
//			// TODO TaskFileName should be ArchiveURL once we update the schema.
//			OrderKeys: "ARRAY_LENGTH(Snapshots) DESC, ParseInfo.TaskFileName, ",
//		}, nil
	default:
		return nil, ErrDatatypeNotSupported
	}
}

var queryTemplates = map[string]*template.Template{
	"dedup": dedupTemplate,
}

// MakeQuery creates a query from a template.
func (params queryer) makeQuery(t *template.Template) string {
	out := bytes.NewBuffer(nil)
	err := t.Execute(out, params)
	if err != nil {
		log.Println(err)
	}
	return out.String()
}

// DedupQuery returns the appropriate query in string form.
func (params queryer) DedupQuery() string {
	return params.makeQuery(dedupTemplate)
}

// Run executes a query constructed from a template.  It returns the bqiface.Job.
func (params queryer) Dedup(ctx context.Context, dryRun bool) (bqiface.Job, error) {
	qs := params.DedupQuery()
	if len(qs) == 0 {
		return nil, dataset.ErrNilQuery
	}
	if params.client == nil {
		return nil, dataset.ErrNilBqClient
	}
	q := params.client.Query(qs)
	if q == nil {
		return nil, dataset.ErrNilQuery
	}
	if dryRun {
		qc := bqiface.QueryConfig{QueryConfig: bigquery.QueryConfig{DryRun: dryRun, Q: qs}}
		q.SetQueryConfig(qc)
	}
	return q.Run(ctx)
}

// CopyToRaw copies the tmp_ job partition to the raw_ job partition.
func (params queryer) CopyToRaw(ctx context.Context, dryRun bool) (bqiface.Job, error) {
	if params.client == nil {
		return nil, dataset.ErrNilBqClient
	}
	// TODO - names should be fields in queryer.
	src := params.client.Dataset("tmp_" + params.Job.Experiment).Table(params.Job.Datatype)
	dest := params.client.Dataset("raw_" + params.Job.Experiment).Table(params.Job.Datatype)

	copier := dest.CopierFrom(src)
	config := bqiface.CopyConfig{}
	config.WriteDisposition = bigquery.WriteTruncate
	config.Dst = dest
	config.Srcs = append(config.Srcs, src)
	copier.SetCopyConfig(config)
	return copier.Run(ctx)
}

// DeleteTmp deletes the tmp table partition.
func (params queryer) DeleteTmp(ctx context.Context) error {
	if params.client == nil {
		return dataset.ErrNilBqClient
	}
	// TODO - name should be field in queryer.
	tmp := params.client.Dataset("tmp_" + params.Job.Experiment).Table(
		fmt.Sprintf("%s$%s", params.Job.Datatype, params.Job.Date.Format("20060102")))
	log.Println("Deleting", tmp.FullyQualifiedName())
	return tmp.Delete(ctx)
}

// TODO get the tmp_ and raw_ from the job Target?
const tmpTable = "`{{.Project}}.tmp_{{.Job.Experiment}}.{{.Job.Datatype}}`"
const rawTable = "`{{.Project}}.raw_{{.Job.Experiment}}.{{.Job.Datatype}}`"

var dedupTemplate = template.Must(template.New("").Parse(`
#standardSQL
# Delete all duplicate rows based on key and prefered priority ordering.
# This is resource intensive for tcpinfo - 20 slot hours for 12M rows with 250M snapshots,
# roughly proportional to the memory footprint of the table partition.
# The query is very cheap if there are no duplicates.
DELETE
FROM ` + tmpTable + ` AS target
WHERE {{.Date}} = "{{.Job.Date.Format "2006-01-02"}}"
# This identifies all rows that don't match rows to preserve.
AND NOT EXISTS (
  # This creates list of rows to preserve, based on key and priority.
  WITH keep AS (
  SELECT * EXCEPT(row_number) FROM (
    SELECT
      {{range $k, $v := .PartitionKeys}}{{$v}}, {{end}}
	  parser.Time,
      ROW_NUMBER() OVER (
        PARTITION BY {{range $k, $v := .PartitionKeys}}{{$v}}, {{end}}date
        ORDER BY {{.OrderKeys}} parser.Time DESC
      ) row_number
      FROM (
        SELECT * FROM ` + tmpTable + `
        WHERE {{.Date}} = "{{.Job.Date.Format "2006-01-02"}}"
      )
    )
    WHERE row_number = 1
  )
  SELECT * FROM keep
  # This matches against the keep table based on keys.  Sufficient select keys must be
  # used to distinguish the preferred row from the others.
  WHERE
    {{range $k, $v := .PartitionKeys}}target.{{$v}} = keep.{{$k}} AND {{end}}
    target.parser.Time = keep.Time
)`))
