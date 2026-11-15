# Job schema

`job.schema.json` defines a job listing as stored in the `active_jobs` MongoDB collection: its fields, which are nullable, and the values of every enum. `generate.mjs` turns it into the types each app uses.

| App | Generated file | Contents |
|---|---|---|
| `apps/service` (Go) | `internal/models/listing.gen.go` | `Listing`, `Company` and the enum types with their constants |
| `apps/website` (TypeScript) | `src/types/job.gen.ts` | `ListingDocument`, `CompanyDocument`, enum types and `*_VALUES` lists |
| `apps/discord-bot` (Python) | `src/backend/mongo/collections/job_schema.py` | enum `Literal` types, `*_VALUES` tuples, `LISTING_FIELDS`, `LISTING_COLLECTION` |

## Changing the job model

1. Edit `job.schema.json`.
2. Run the generator from the repository root. It needs Node 22+ and `gofmt`.

   ```sh
   node schema/generate.mjs
   ```

3. Commit the schema and the generated files together, then update the code that uses the new field or value.

Never edit a generated file by hand. CI runs `node schema/generate.mjs --check` and fails when one is out of date.

## Schema extensions

Standard JSON Schema keywords describe the document. These extra keys steer the generator:

| Key | On | Meaning |
|---|---|---|
| `x-collection` | root | MongoDB collection name |
| `x-format: "object-id"` | property | a MongoDB ObjectId |
| `x-go.name` | property | Go field name, when it is not the PascalCase of the key |
| `x-go.omitempty` | property | adds `omitempty` to the `bson` tag |
| `x-go.json` | property | `json` tag, when it differs from the key |
| `x-go.skip` | property | leaves the field out of the Go struct |
| `x-enum-descriptions` | enum | comment for a single enum value |

A property typed `["string", "null"]`, or `anyOf` with `null`, is a pointer in Go, `T | null` in TypeScript. A property missing from `required` is optional in TypeScript.
