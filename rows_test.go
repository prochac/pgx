package pgx_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testRowScanner struct {
	name string
	age  int32
}

func (rs *testRowScanner) ScanRow(rows pgx.Rows) error {
	return rows.Scan(&rs.name, &rs.age)
}

func TestRowScanner(t *testing.T) {
	t.Parallel()

	defaultConnTestRunner.RunTest(context.Background(), t, func(ctx context.Context, t testing.TB, conn *pgx.Conn) {
		var s testRowScanner
		err := conn.QueryRow(ctx, "select 'Adam' as name, 72 as height").Scan(&s)
		require.NoError(t, err)
		require.Equal(t, "Adam", s.name)
		require.Equal(t, int32(72), s.age)
	})
}

func TestForEachRow(t *testing.T) {
	t.Parallel()

	pgxtest.RunWithQueryExecModes(context.Background(), t, defaultConnTestRunner, nil, func(ctx context.Context, t testing.TB, conn *pgx.Conn) {
		var actualResults []any

		rows, _ := conn.Query(
			context.Background(),
			"select n, n * 2 from generate_series(1, $1) n",
			3,
		)
		var a, b int
		ct, err := pgx.ForEachRow(rows, []any{&a, &b}, func() error {
			actualResults = append(actualResults, []any{a, b})
			return nil
		})
		require.NoError(t, err)

		expectedResults := []any{
			[]any{1, 2},
			[]any{2, 4},
			[]any{3, 6},
		}
		require.Equal(t, expectedResults, actualResults)
		require.EqualValues(t, 3, ct.RowsAffected())
	})
}

func TestForEachRowScanError(t *testing.T) {
	t.Parallel()

	pgxtest.RunWithQueryExecModes(context.Background(), t, defaultConnTestRunner, nil, func(ctx context.Context, t testing.TB, conn *pgx.Conn) {
		var actualResults []any

		rows, _ := conn.Query(
			context.Background(),
			"select 'foo', 'bar' from generate_series(1, $1) n",
			3,
		)
		var a, b int
		ct, err := pgx.ForEachRow(rows, []any{&a, &b}, func() error {
			actualResults = append(actualResults, []any{a, b})
			return nil
		})
		require.EqualError(t, err, "can't scan into dest[0]: cannot scan text (OID 25) in text format into *int")
		require.Equal(t, pgconn.CommandTag{}, ct)
	})
}

func TestForEachRowAbort(t *testing.T) {
	t.Parallel()

	pgxtest.RunWithQueryExecModes(context.Background(), t, defaultConnTestRunner, nil, func(ctx context.Context, t testing.TB, conn *pgx.Conn) {
		rows, _ := conn.Query(
			context.Background(),
			"select n, n * 2 from generate_series(1, $1) n",
			3,
		)
		var a, b int
		ct, err := pgx.ForEachRow(rows, []any{&a, &b}, func() error {
			return errors.New("abort")
		})
		require.EqualError(t, err, "abort")
		require.Equal(t, pgconn.CommandTag{}, ct)
	})
}

func ExampleForEachRow() {
	conn, err := pgx.Connect(context.Background(), os.Getenv("PGX_TEST_DATABASE"))
	if err != nil {
		fmt.Printf("Unable to establish connection: %v", err)
		return
	}

	rows, _ := conn.Query(
		context.Background(),
		"select n, n * 2 from generate_series(1, $1) n",
		3,
	)
	var a, b int
	_, err = pgx.ForEachRow(rows, []any{&a, &b}, func() error {
		fmt.Printf("%v, %v\n", a, b)
		return nil
	})
	if err != nil {
		fmt.Printf("ForEachRow error: %v", err)
		return
	}

	// Output:
	// 1, 2
	// 2, 4
	// 3, 6
}

func TestCollectRows(t *testing.T) {
	defaultConnTestRunner.RunTest(context.Background(), t, func(ctx context.Context, t testing.TB, conn *pgx.Conn) {
		rows, _ := conn.Query(ctx, `select n from generate_series(0, 99) n`)
		numbers, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (int32, error) {
			var n int32
			err := row.Scan(&n)
			return n, err
		})
		require.NoError(t, err)

		assert.Len(t, numbers, 100)
		for i := range numbers {
			assert.Equal(t, int32(i), numbers[i])
		}
	})
}

// This example uses CollectRows with a manually written collector function. In most cases RowTo, RowToAddrOf,
// RowToStructByPos, RowToAddrOfStructByPos, or another generic function would be used.
func ExampleCollectRows() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, os.Getenv("PGX_TEST_DATABASE"))
	if err != nil {
		fmt.Printf("Unable to establish connection: %v", err)
		return
	}

	rows, _ := conn.Query(ctx, `select n from generate_series(1, 5) n`)
	numbers, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (int32, error) {
		var n int32
		err := row.Scan(&n)
		return n, err
	})
	if err != nil {
		fmt.Printf("CollectRows error: %v", err)
		return
	}

	fmt.Println(numbers)

	// Output:
	// [1 2 3 4 5]
}

func TestCollectOneRow(t *testing.T) {
	defaultConnTestRunner.RunTest(context.Background(), t, func(ctx context.Context, t testing.TB, conn *pgx.Conn) {
		rows, _ := conn.Query(ctx, `select 42`)
		n, err := pgx.CollectOneRow(rows, func(row pgx.CollectableRow) (int32, error) {
			var n int32
			err := row.Scan(&n)
			return n, err
		})
		assert.NoError(t, err)
		assert.Equal(t, int32(42), n)
	})
}

func TestCollectOneRowNotFound(t *testing.T) {
	defaultConnTestRunner.RunTest(context.Background(), t, func(ctx context.Context, t testing.TB, conn *pgx.Conn) {
		rows, _ := conn.Query(ctx, `select 42 where false`)
		n, err := pgx.CollectOneRow(rows, func(row pgx.CollectableRow) (int32, error) {
			var n int32
			err := row.Scan(&n)
			return n, err
		})
		assert.ErrorIs(t, err, pgx.ErrNoRows)
		assert.Equal(t, int32(0), n)
	})
}

func TestCollectOneRowIgnoresExtraRows(t *testing.T) {
	defaultConnTestRunner.RunTest(context.Background(), t, func(ctx context.Context, t testing.TB, conn *pgx.Conn) {
		rows, _ := conn.Query(ctx, `select n from generate_series(42, 99) n`)
		n, err := pgx.CollectOneRow(rows, func(row pgx.CollectableRow) (int32, error) {
			var n int32
			err := row.Scan(&n)
			return n, err
		})
		require.NoError(t, err)

		assert.NoError(t, err)
		assert.Equal(t, int32(42), n)
	})
}

func TestRowTo(t *testing.T) {
	defaultConnTestRunner.RunTest(context.Background(), t, func(ctx context.Context, t testing.TB, conn *pgx.Conn) {
		rows, _ := conn.Query(ctx, `select n from generate_series(0, 99) n`)
		numbers, err := pgx.CollectRows(rows, pgx.RowTo[int32])
		require.NoError(t, err)

		assert.Len(t, numbers, 100)
		for i := range numbers {
			assert.Equal(t, int32(i), numbers[i])
		}
	})
}

func ExampleRowTo() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, os.Getenv("PGX_TEST_DATABASE"))
	if err != nil {
		fmt.Printf("Unable to establish connection: %v", err)
		return
	}

	rows, _ := conn.Query(ctx, `select n from generate_series(1, 5) n`)
	numbers, err := pgx.CollectRows(rows, pgx.RowTo[int32])
	if err != nil {
		fmt.Printf("CollectRows error: %v", err)
		return
	}

	fmt.Println(numbers)

	// Output:
	// [1 2 3 4 5]
}

func TestRowToAddrOf(t *testing.T) {
	defaultConnTestRunner.RunTest(context.Background(), t, func(ctx context.Context, t testing.TB, conn *pgx.Conn) {
		rows, _ := conn.Query(ctx, `select n from generate_series(0, 99) n`)
		numbers, err := pgx.CollectRows(rows, pgx.RowToAddrOf[int32])
		require.NoError(t, err)

		assert.Len(t, numbers, 100)
		for i := range numbers {
			assert.Equal(t, int32(i), *numbers[i])
		}
	})
}

func ExampleRowToAddrOf() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, os.Getenv("PGX_TEST_DATABASE"))
	if err != nil {
		fmt.Printf("Unable to establish connection: %v", err)
		return
	}

	rows, _ := conn.Query(ctx, `select n from generate_series(1, 5) n`)
	pNumbers, err := pgx.CollectRows(rows, pgx.RowToAddrOf[int32])
	if err != nil {
		fmt.Printf("CollectRows error: %v", err)
		return
	}

	for _, p := range pNumbers {
		fmt.Println(*p)
	}

	// Output:
	// 1
	// 2
	// 3
	// 4
	// 5
}

func TestRowToMap(t *testing.T) {
	defaultConnTestRunner.RunTest(context.Background(), t, func(ctx context.Context, t testing.TB, conn *pgx.Conn) {
		rows, _ := conn.Query(ctx, `select 'Joe' as name, n as age from generate_series(0, 9) n`)
		slice, err := pgx.CollectRows(rows, pgx.RowToMap)
		require.NoError(t, err)

		assert.Len(t, slice, 10)
		for i := range slice {
			assert.Equal(t, "Joe", slice[i]["name"])
			assert.EqualValues(t, i, slice[i]["age"])
		}
	})
}

func TestRowToStructByPos(t *testing.T) {
	type person struct {
		Name string
		Age  int32
	}

	defaultConnTestRunner.RunTest(context.Background(), t, func(ctx context.Context, t testing.TB, conn *pgx.Conn) {
		rows, _ := conn.Query(ctx, `select 'Joe' as name, n as age from generate_series(0, 9) n`)
		slice, err := pgx.CollectRows(rows, pgx.RowToStructByPos[person])
		require.NoError(t, err)

		assert.Len(t, slice, 10)
		for i := range slice {
			assert.Equal(t, "Joe", slice[i].Name)
			assert.EqualValues(t, i, slice[i].Age)
		}
	})
}

func ExampleRowToStructByPos() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, os.Getenv("PGX_TEST_DATABASE"))
	if err != nil {
		fmt.Printf("Unable to establish connection: %v", err)
		return
	}

	if conn.PgConn().ParameterStatus("crdb_version") != "" {
		// Skip test / example when running on CockroachDB. Since an example can't be skipped fake success instead.
		fmt.Println(`Cheeseburger: $10
Fries: $5
Soft Drink: $3`)
		return
	}

	// Setup example schema and data.
	_, err = conn.Exec(ctx, `
create temporary table products (
	id int primary key generated by default as identity,
	name varchar(100) not null,
	price int not null
);

insert into products (name, price) values
	('Cheeseburger', 10),
	('Double Cheeseburger', 14),
	('Fries', 5),
	('Soft Drink', 3);
`)
	if err != nil {
		fmt.Printf("Unable to setup example schema and data: %v", err)
		return
	}

	type product struct {
		ID    int32
		Name  string
		Price int32
	}

	rows, _ := conn.Query(ctx, "select * from products where price < $1 order by price desc", 12)
	products, err := pgx.CollectRows(rows, pgx.RowToStructByPos[product])
	if err != nil {
		fmt.Printf("CollectRows error: %v", err)
		return
	}

	for _, p := range products {
		fmt.Printf("%s: $%d\n", p.Name, p.Price)
	}

	// Output:
	// Cheeseburger: $10
	// Fries: $5
	// Soft Drink: $3
}

func TestRowToAddrOfStructPos(t *testing.T) {
	type person struct {
		Name string
		Age  int32
	}

	defaultConnTestRunner.RunTest(context.Background(), t, func(ctx context.Context, t testing.TB, conn *pgx.Conn) {
		rows, _ := conn.Query(ctx, `select 'Joe' as name, n as age from generate_series(0, 9) n`)
		slice, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByPos[person])
		require.NoError(t, err)

		assert.Len(t, slice, 10)
		for i := range slice {
			assert.Equal(t, "Joe", slice[i].Name)
			assert.EqualValues(t, i, slice[i].Age)
		}
	})
}