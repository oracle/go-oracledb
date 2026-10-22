# Working with Oracle JSON

## Oracle's native JSON type and OSON

Oracle Database 21c introduced the native SQL `JSON` data type. The native type stores documents in
[OSON (Oracle Binary JSON)](https://blogs.oracle.com/database/autonomous-json-database-under-the-covers-oson-format),
Oracle Database's optimized binary format, designed for fast queries and
updates and able to represent SQL scalar types such as dates and timestamps.
See the [Oracle JSON Developer's
Guide](https://docs.oracle.com/en/database/oracle/oracle-database/26/adjsn/index.html)
for details about the database type and SQL support.

To create a table with a column named `doc` for JSON data, for example:

```sql
CREATE TABLE json_documents (
    id NUMBER PRIMARY KEY,
    doc JSON
);
```

## JSON support in the driver

The driver supports Oracle Database's native `JSON` type through the
`github.com/oracle/go-oracledb/v26/oracle/datatype/json` package (imported as
`ojson` in the examples below). You can bind Go values or existing JSON text,
fetch a document through `database/sql`, and read
either the entire document or selected fields. Native `JSON` columns require
Oracle Database 21c or later.

## Choose the right representation

The following table summarizes the available APIs for binding and fetching JSON.

| What you have or need | API | Behavior |
| --- | --- | --- |
| A Go object, array, or scalar to bind | `ojson.NewJSON(value)` | Encodes the value as OSON, Oracle's binary JSON format. |
| Go values with explicit conversion options | `ojson.NewJSONWithOptions(value, opts)` | Uses the specified [options](#json-encoding-and-decoding-options) for encoding and decoding. |
| Existing JSON text to bind | `ojson.NewJSONFromString(text)` | Sends the JSON text as a string bind value (not encoded as OSON). |
| A native JSON column to fetch | `ojson.JSON` | Holds the fetched JSON document. |
| A nullable native JSON column | `sql.Null[ojson.JSON]` | Distinguishes SQL NULL from a JSON document containing `null`. |

## Bind a JSON document

To insert a document, use `NewJSON` for Go values, `NewJSONWithOptions` for
custom [options](#json-encoding-and-decoding-options), or `NewJSONFromString` for existing JSON text.

```go
doc, err := ojson.NewJSON(map[string]any{"name": "Alice", "active": true})
if err != nil {
	return fmt.Errorf("create JSON document: %w", err)
}
_, err := db.ExecContext(ctx,
	"INSERT INTO json_documents (id, doc) VALUES (:1, :2)", 1, doc,
)
```

You can bind existing JSON text with `NewJSONFromString`. It is validated at
the bind boundary:

```go
doc := ojson.NewJSONFromString(`{"name":"Alice","active":true}`)
_, err := db.ExecContext(ctx,
	"INSERT INTO json_documents (id, doc) VALUES (:1, :2)", 2, doc,
)
```

## Read a JSON document

Use the `JSON` type to fetch a native JSON column:

```go
var doc ojson.JSON
err := db.QueryRowContext(ctx,
	"SELECT doc FROM json_documents WHERE id = :1", 1,
).Scan(&doc)
```

## JSON access APIs

`JSON` provides the following APIs for accessing a document:

| API | Description |
| --- | --- |
| `GetValue() (any, error)` | Returns the entire document as Go values: `map[string]any` for objects, `[]any` for arrays, and the corresponding Go type for scalars. |
| `AsJSONObject() (JSONObject, error)` | Returns a `JSONObject`. |
| `AsJSONArray() (JSONArray, error)` | Returns a `JSONArray`. |
| `AsJSONScalar() (JSONScalar, error)` | Returns a `JSONScalar`. |
| `Kind() (JSONKind, error)` | Identifies an object, array, or scalar, including JSON null. |

For a `JSON` document fetched from the database, `JSONObject`, `JSONArray`,
and `JSONScalar` work on the underlying OSON bytes and decode values on demand.
Object and array `Get` methods return child `JSON` values. Calling `GetValue()`
decodes the selected value, including all its children if it is an object or
array.

`JSON`, `JSONObject`, `JSONArray`, and `JSONScalar` implement
[json.Marshaler](https://pkg.go.dev/encoding/json#Marshaler). Use
`json.Marshal` to obtain JSON text when rendering errors need to be reported;
the `String` methods are intended for diagnostics and return a placeholder when
rendering fails.

`JSON` also implements [json.Unmarshaler](https://pkg.go.dev/encoding/json#Unmarshaler),
so standard JSON text can be decoded directly into an OSON document. JSON
numbers retain their text representation rather than being converted to
`float64`.

```go
text, err := json.Marshal(doc)
if err != nil {
	return fmt.Errorf("marshal JSON document: %w", err)
}
fmt.Printf("JSON text: %s\n", text)
```

```go
var doc ojson.JSON
if err := json.Unmarshal([]byte(`{"score":9007199254740993}`), &doc); err != nil {
	return fmt.Errorf("decode JSON document: %w", err)
}
```

### JSONObject

Use `AsJSONObject()` to access an object.

| Method | Description |
| --- | --- |
| `Keys() []string` | Returns member names in unspecified order. |
| `Get(key string) (JSON, error)` | Returns a child `JSON`, `JSONObjectKeyNotFoundError` for an absent member, or an error constructing the child node. Keys are literal names. |
| `GetValue() (map[string]any, error)` | Decodes the entire object as `map[string]any`. |
| `String() string` | Returns JSON text (returns a placeholder on failure like unsupported type). |

```go
object, err := doc.AsJSONObject()
keys := object.Keys()
```

### JSONArray

Use `AsJSONArray()` to access an array.

| Method | Description |
| --- | --- |
| `Len() int` | Returns the number of elements. |
| `Get(i int) (JSON, error)` | Returns a child `JSON` at a zero-based index, or an error if the index is out of range or the child node cannot be constructed. |
| `GetValue() ([]any, error)` | Decodes the entire array as `[]any`. |
| `String() string` | Returns JSON text (returns a placeholder on failure like unsupported type). |

```go
array, err := doc.AsJSONArray()
length := array.Len()
```

### JSONScalar

Use `AsJSONScalar()` to access a scalar, including JSON null.

| Method | Description |
| --- | --- |
| `GetValue() (any, error)` | Decodes the scalar as its corresponding Go value; JSON null becomes `nil`. |
| `String() string` | Returns JSON text (returns a placeholder on failure like unsupported type). |

```go
scalar, err := doc.AsJSONScalar()
text := scalar.String()
```

## Type Mapping

The following table summarizes the driver's binding and fetching type mappings.
`stdjson` refers to Go's `encoding/json` package. Fetched JSON values are
converted by `GetValue()` using the default options; SQL `NULL` is handled when
fetching the column.

| JSON Value | Go Value for Binding | Go Value when Fetching |
| --- | --- | --- |
| object | `map[string]any` | `map[string]any` |
| array | `[]any` | `[]any` |
| string | `string` | `string` |
| number | `int`, `int8`, `int16`, `int32`, `int64`, `uint`, `uint8`, `uint16`, `uint32`, `uint64`, `float32`, `float64`, `stdjson.Number` | `int32`, `int64`, `stdjson.Number`, or `float64`; see [Number mapping](#number-mapping). |
| `true` | `true` | `true` |
| `false` | `false` | `false` |
| `RAW` | `[]byte` | `[]byte` |
| `ID` | Not supported | `[]byte` |
| `TIMESTAMP` (default time encoding) | `time.Time` | `time.Time` |
| `DATE` | `time.Time` with `TimeAsDate` option | `time.Time` |
| `TIMESTAMP WITH TIME ZONE` | `time.Time` with `TimeAsTimestampTZ` option | `time.Time` |
| `INTERVAL YEAR TO MONTH` | Not supported | `string` |
| `INTERVAL DAY TO SECOND` | Not supported | `string` |
| JSON `null` | `nil` | `nil` |
| SQL `NULL` | `nil` passed directly as a SQL parameter | `sql.Null[ojson.JSON]` with `Valid` set to `false` |

### Number mapping

When binding, the Go number type determines the OSON number type. With the
default `NumberMode`, the following table shows what `GetValue()` returns after
fetching it:

| Go value bound | Stored OSON number | Default Go value fetched |
| --- | --- | --- |
| `int8`, `int16`, `int32`, `uint8`, `uint16` | Signed 32-bit integer | `int32` |
| `int`, `int64`, `uint32` | Signed 64-bit integer | `int64` |
| `uint`, `uint64` | Oracle `NUMBER` | `stdjson.Number` |
| `stdjson.Number` | Number stored as text | `stdjson.Number` |
| `float32` | `BINARY_FLOAT` | `float64` |
| `float64` | `BINARY_DOUBLE` | `float64` |

To change the document-wide number-decoding behavior, set `Options.NumberMode` to
`NumberAsJSONNumber` for `stdjson.Number` values or `NumberAsFloat64` for
`float64` values. Leave it as `NumberDefault` to use the mapping above.
Note: special binary float values like `NaN` remain `float64` in every mode.

See [Control number representation](#control-number-representation) for how to set the option.

## JSON encoding and decoding options

`Options` lets you control how Go values are encoded into JSON and how JSON
values are decoded into Go values. Use it to customize these conversions for
your application. Each option applies to the whole document, not to individual values.

### Control number representation
The `NumberMode` option selects the Go type returned when reading JSON numbers with
`GetValue()`. It applies to every number in the document.

The default mode already preserves each number in an appropriate Go type. Set
`NumberMode` only when your application needs every JSON number to have the
same Go representation.

| Option | Go representation |
| --- | --- |
| `NumberDefault` (default) | `int32` or `int64` for compact signed integers; `stdjson.Number` for Oracle NUMBER and string numbers; `float64` for binary floats. |
| `NumberAsJSONNumber` | `stdjson.Number` for finite numbers; `float64` for NaN and ±Inf. |
| `NumberAsFloat64` | `float64` for all numbers. |

Converting to `float64` can round large integers and decimal values.
`NumberAsJSONNumber` preserves decimal numbers but cannot recover precision
already lost in a Go floating-point value.

For example, a row contains this JSON:

```json
{"score": 9007199254740993}
```

Set `NumberAsJSONNumber` when every finite number should be returned as
`stdjson.Number` (or set `NumberAsFloat64` when every number should be a
`float64`):

```go
var doc ojson.JSON
doc.SetOptions(ojson.Options{
	NumberMode: ojson.NumberAsJSONNumber,
})

if err := db.QueryRowContext(ctx,
	"SELECT doc FROM json_documents WHERE id = :1", 1,
).Scan(&doc); err != nil {
	return err
}
value, err := doc.GetValue()
```

Expected `value`:

```go
map[string]any{"score": stdjson.Number("9007199254740993")}
```

### Control time encoding

`TimeEncoding` selects the OSON scalar used to store `time.Time` values when
constructing JSON. It applies to every `time.Time` value in the document.

| Option | Stored OSON scalar | Preserved information |
| --- | --- | --- |
| `TimeAsTimestamp` (default) | `TIMESTAMP` | Date and time, including nanoseconds. |
| `TimeAsTimestampTZ` | `TIMESTAMP WITH TIME ZONE` | Date and time, including nanoseconds and the numeric UTC offset. |
| `TimeAsDate` | `DATE` | Date and time through whole seconds. |

`TimeAsTimestamp` and `TimeAsDate` omit the time zone, `TimeAsDate` also drops
fractional seconds and `TimeAsTimestampTZ` keeps the numeric offset.

For example, encode this `time.Time` with each option:

```go
createdAt := time.Date(2025, 2, 3, 4, 5, 6, 123456789, time.FixedZone("", 2*60*60))
// createdAt is 2025-02-03T04:05:06.123456789+02:00
doc, err := ojson.NewJSONWithOptions(createdAt, ojson.Options{
	TimeEncoding: ojson.TimeAsTimestampTZ,
})
if err != nil {
	return fmt.Errorf("create JSON timestamp: %w", err)
}
```

The OSON scalar stored in `doc` for each option:

| TimeEncoding | Stored OSON scalar | Stored value for `createdAt` |
| --- | --- | --- |
| `TimeAsTimestamp` (default) | `TIMESTAMP` | `2025-02-03 04:05:06.123456789` |
| `TimeAsTimestampTZ` | `TIMESTAMP WITH TIME ZONE` | `2025-02-03 04:05:06.123456789 +02:00` |
| `TimeAsDate` | `DATE` | `2025-02-03 04:05:06` |

## Caveats

### Get JSON text with `json.Marshal`

Use `json.Marshal` for JSON text that your application sends, stores, or
processes. It returns an error if the document cannot be rendered:

```go
text, err := json.Marshal(doc)
if err != nil {
	return fmt.Errorf("marshal JSON document: %w", err)
}
```

`String()` is intended for diagnostics. It cannot return an error, so it
returns a sentinel instead of JSON text:

- `"<JSON: uninitialized>"` (same pattern for the other types) for the zero
  value, such as a `JSON` that was never fetched or bound.
- `"<JSON: rendering failed>"` when rendering fails, for example when the
  document contains an unsupported OSON scalar (see below) or a malformed payload.

Rendering a container fails as a whole if any child in that subtree cannot be
rendered. Use `json.Marshal` when the error matters.

### Unsupported scalar types

The driver decodes the JSON scalars (null, boolean, string, numbers including
Oracle NUMBER, `BINARY_FLOAT`, and `BINARY_DOUBLE`) and the
SQL/JSON scalar extensions `DATE`, `TIMESTAMP`, `TIMESTAMP WITH TIME ZONE`,
`BINARY`/`RAW`, `ID`, `INTERVAL YEAR TO MONTH`, and `INTERVAL DAY TO SECOND`.

The following OSON scalars are recognized but not implemented yet. Accessing a
document that contains one with `GetValue()` returns an error; rendering the
affected subtree with `String()` returns `"<JSON: rendering failed>"`:

- Native integer
- Extended binary values: vectors, scalar arrays, and embedded OSON

### Types not supported for binding

- `INTERVAL YEAR TO MONTH`, `INTERVAL DAY TO SECOND`, and `ID` scalars are
  fetch-only; no Go type binds to them.
- Only these Go types can be encoded: `nil`, `bool`, `string`, `int`/`uint` of
  every width, `float32`, `float64`, `[]byte`, `time.Time`, `stdjson.Number`,
  `map[string]any`, and `[]any`. Therefore, other types like structs, pointers, named types,
  `[]string`, maps with non-`string` keys will fail with an encoding error.

### Nil values

`NewJSON(nil)` creates JSON null. Typed nil values retain their Go kind:
`map[string]any(nil)` creates `{}`, `[]any(nil)` creates `[]`, and `[]byte(nil)`
creates an empty binary scalar that renders as `""`. Passing `nil` directly as a
SQL bind parameter creates SQL NULL. Use `sql.Null[ojson.JSON]` to distinguish
SQL NULL from JSON null when fetching.

### Decoded time zone

`DATE` and `TIMESTAMP` payloads carry no zone, so the driver materializes them
as `time.Time` in Go's local zone (`time.Local`). `TIMESTAMP WITH TIME ZONE`
keeps only the numeric UTC offset. Intervals are materialized as `string`.

### Binary rendering

`[]byte` values (`BINARY`/`RAW`) render as uppercase hexadecimal JSON strings,
not as `encoding/json`'s base64.
