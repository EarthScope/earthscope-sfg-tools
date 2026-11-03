### Parsing Level

Functions take in scope (optionally), log type, table def structure.

Functions return an arrow table.

### Write Level

Functions take in an arrow table, table def, and catalog instance.

These functions write the arrow table to the catalog. 

We could also include functions that write straight to csv/parquet.


### Rules

1. 1 function call per file/log type/table schema instances
2. 1 parser instance per source file type/schema type
3. parser instances have an "EDID" factory function method
3. file path,scope/log type, are passed as arguments to the parsing function
4. parsing functions take in file path + log type. EDID columns are added after the parsing 
function returns an arrow table. THis logic is encapsulated in the parser method.