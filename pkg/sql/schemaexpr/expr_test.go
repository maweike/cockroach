// Copyright 2020 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package schemaexpr

import (
	"context"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/sql/parser"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/builtins"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/tree"
	"github.com/cockroachdb/cockroach/pkg/sql/types"
)

func TestValidateExpr(t *testing.T) {
	ctx := context.Background()
	semaCtx := tree.MakeSemaContext()

	// Trick to get the init() for the builtins package to run.
	_ = builtins.AllBuiltinNames

	database := tree.Name("foo")
	table := tree.Name("bar")
	tn := tree.MakeTableName(database, table)

	desc := testTableDesc(
		string(table),
		[]testCol{{"a", types.Bool}, {"b", types.Int}},
		[]testCol{{"c", types.String}},
	)

	testData := []struct {
		expr          string
		expectedValid bool
		expectedExpr  string
		typ           *types.T
		maxVolatility tree.Volatility
	}{
		// De-qualify column names.
		{"bar.a", true, "a", types.Bool, tree.VolatilityImmutable},
		{"foo.bar.a", true, "a", types.Bool, tree.VolatilityImmutable},
		{"bar.b = 0", true, "b = 0:::INT8", types.Bool, tree.VolatilityImmutable},
		{"foo.bar.b = 0", true, "b = 0:::INT8", types.Bool, tree.VolatilityImmutable},
		{"bar.a AND foo.bar.b = 0", true, "a AND (b = 0:::INT8)", types.Bool, tree.VolatilityImmutable},

		// Validates the type of the expression.
		{"concat(c, c)", true, "concat(c, c)", types.String, tree.VolatilityImmutable},
		{"concat(c, c)", false, "", types.Int, tree.VolatilityImmutable},
		{"b + 1", true, "b + 1:::INT8", types.Int, tree.VolatilityImmutable},
		{"b + 1", false, "", types.Bool, tree.VolatilityImmutable},

		// Validates that the expression has no variable expressions.
		{"$1", false, "", types.Any, tree.VolatilityImmutable},

		// Validates the volatility check.
		{"now()", true, "now():::TIMESTAMPTZ", types.TimestampTZ, tree.VolatilityVolatile},
		{"now()", true, "now():::TIMESTAMPTZ", types.TimestampTZ, tree.VolatilityStable},
		{"now()", false, "", types.Any, tree.VolatilityImmutable},
		{"uuid_v4()::STRING", true, "uuid_v4()::STRING", types.String, tree.VolatilityVolatile},
		{"uuid_v4()::STRING", false, "", types.String, tree.VolatilityStable},
		{"uuid_v4()::STRING", false, "", types.String, tree.VolatilityImmutable},
	}

	for _, d := range testData {
		t.Run(d.expr, func(t *testing.T) {
			expr, err := parser.ParseExpr(d.expr)
			if err != nil {
				t.Fatalf("%s: unexpected error: %s", d.expr, err)
			}

			expr, _, err = DequalifyAndValidateExpr(
				ctx,
				&desc,
				expr,
				d.typ,
				"test-validate-expr",
				&semaCtx,
				d.maxVolatility,
				&tn,
			)

			if !d.expectedValid {
				if err == nil {
					t.Fatalf("%s: expected invalid expression, but was valid", d.expr)
				}
				// The input expression is invalid so there is no need to check
				// the output expression r.
				return
			}

			if err != nil {
				t.Fatalf("%s: expected valid expression, but found error: %s", d.expr, err)
			}

			s := tree.Serialize(expr)
			if s != d.expectedExpr {
				t.Errorf("%s: expected %q, got %q", d.expr, d.expectedExpr, s)
			}
		})
	}
}
