package logicalplan

import (
	"context"
	"testing"

	"github.com/apache/arrow/go/v8/arrow"
	"github.com/apache/arrow/go/v8/arrow/memory"
	"github.com/stretchr/testify/require"

	"github.com/polarsignals/frostdb/dynparquet"
)

type mockTableReader struct {
	schema *dynparquet.Schema
}

func (m *mockTableReader) Schema() *dynparquet.Schema {
	return m.schema
}

func (m *mockTableReader) View(ctx context.Context, fn func(ctx context.Context, tx uint64) error) error {
	return nil
}

func (m *mockTableReader) Iterator(
	ctx context.Context,
	tx uint64,
	pool memory.Allocator,
	schema *arrow.Schema,
	physicalProjection []Expr,
	projection []Expr,
	filter Expr,
	distinctColumns []Expr,
	callback func(ctx context.Context, r arrow.Record) error,
) error {
	return nil
}

func (m *mockTableReader) SchemaIterator(
	ctx context.Context,
	tx uint64,
	pool memory.Allocator,
	physicalProjection []Expr,
	projection []Expr,
	filter Expr,
	distinctColumns []Expr,
	callback func(ctx context.Context, r arrow.Record) error,
) error {
	return nil
}

func (m *mockTableReader) ArrowSchema(
	ctx context.Context,
	tx uint64,
	pool memory.Allocator,
	physicalProjection []Expr,
	projection []Expr,
	filter Expr,
	distinctColumns []Expr,
) (*arrow.Schema, error) {
	return nil, nil
}

type mockTableProvider struct {
	schema *dynparquet.Schema
}

func (m *mockTableProvider) GetTable(name string) TableReader {
	return &mockTableReader{
		schema: m.schema,
	}
}

func TestInputSchemaGetter(t *testing.T) {
	schema := dynparquet.NewSampleSchema()

	// test we can get the table by traversing to find the TableScan
	plan, _ := (&Builder{}).
		Scan(&mockTableProvider{schema}, "table1").
		Filter(Col("labels.test").Eq(Literal("abc"))).
		Aggregate(
			Sum(Col("value")).Alias("value_sum"),
			Col("stacktrace"),
		).
		Project(Col("stacktrace")).
		Build()
	require.Equal(t, schema, plan.InputSchema())

	// test we can get the table by traversing to find SchemaScan
	plan, _ = (&Builder{}).
		ScanSchema(&mockTableProvider{schema}, "table1").
		Filter(Col("labels.test").Eq(Literal("abc"))).
		Aggregate(
			Sum(Col("value")).Alias("value_sum"),
			Col("stacktrace"),
		).
		Project(Col("stacktrace")).
		Build()
	require.Equal(t, schema, plan.InputSchema())

	// test it returns null in case where we built a logical plan w/ no
	// TableScan or SchemaScan
	plan, _ = (&Builder{}).
		Filter(Col("labels.test").Eq(Literal("abc"))).
		Aggregate(
			Sum(Col("value")).Alias("value_sum"),
			Col("stacktrace"),
		).
		Project(Col("stacktrace")).
		Build()
	require.Nil(t, plan.InputSchema())
}
