# Oracle JSON example

This directory contains a runnable Go sample that uses Oracle Database's native `JSON` type. It inserts one document from JSON text and another from Go values, then fetches both and reads selected fields.

The example requires Oracle Database 21c or later, or a database version that supports the native SQL `JSON` data type.

## Run the example

From the repository root, set `ORACLE_DSN` to your database connection string and run:

```bash
export ORACLE_DSN='user/password@localhost:1521/freepdb1'
go run ./examples/json
```

See the [driver documentation](../../README.md#data-source-name-as-a-string) for connection string details.

## What the example demonstrates

### Create the table

The sample creates a table with a row ID and a native JSON document column:

```sql
CREATE TABLE go_driver_json_example (
    id NUMBER PRIMARY KEY,
    doc JSON
);
```

### Insert JSON text and Go values

The sample inserts two JSON documents with the same structure: one from JSON text using [`ojson.NewJSONFromString`](../../documentation/json.md#bind-a-json-document), and one from Go maps and slices using [`ojson.NewJSON`](../../documentation/json.md#bind-a-json-document).

First document:

```json
{
  "order": {
    "id": 9007199254740993,
    "items": [
      {"sku": "A-100"}
    ]
  }
}
```

Second document:

```json
{
  "order": {
    "id": 9007199254740994,
    "items": [
      {"sku": "C-300"}
    ]
  }
}
```

### Fetch and convert to JSON text

The sample fetches both rows and scans each native JSON document into an [`ojson.JSON`](../../documentation/json.md#read-a-json-document) value.

It then calls `json.Marshal` on that value to convert the native JSON document to JSON text. See [Get JSON text with `json.Marshal`](../../documentation/json.md#get-json-text-with-json-marshal) for more details.

### Access nested JSON values

The sample sets `NumberMode` to `ojson.NumberAsJSONNumber`, so `GetValue()` returns every number in the fetched document as `json.Number`. See [JSON number options](../../documentation/json.md#control-number-representation) for details.

The example accesses these paths in the first JSON document:

```text
doc (object)
└── order (object)
    ├── id (number): 9007199254740993
    └── items (array, 1 element)
        └── [0] (object)
            └── sku (string): "A-100"
```

1. **Root and order:** [`AsJSONObject`](../../documentation/json.md#jsonobject) views the document's root object. [`Get("order")`](../../documentation/json.md#jsonobject) selects its `order` member, and `AsJSONObject` views that member as an object.

2. **Order ID:** `Get("id")` selects the ID from the order object. [`AsJSONScalar`](../../documentation/json.md#jsonscalar) and [`GetValue()`](../../documentation/json.md#jsonscalar) return `9007199254740993` as a `json.Number`.

3. **Items array:** `Get("items")` selects the items from the order object. [`AsJSONArray`](../../documentation/json.md#jsonarray) views them as an array, and [`Len()`](../../documentation/json.md#jsonarray) returns `1`.

4. **First item's SKU:** [`Get(0)`](../../documentation/json.md#jsonarray) selects the first item. `AsJSONObject` views it as an object; `Get("sku")`, `AsJSONScalar`, and `GetValue()` return `A-100`.

See [Working with Oracle JSON](../../documentation/json.md) for more information about the JSON APIs and number options.
