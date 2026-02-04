# Multi-File Excel Upload with Automatic Relationship Detection

## Overview

This feature allows you to upload multiple related Excel files at once, and the system will automatically detect and configure the relationships between them.

## Endpoint

```
POST /:tenantSlug/:projectSlug/excel/upload-multiple
```

## How It Works

### 1. Automatic Relationship Detection

The system analyzes all uploaded files to detect relationships based on:

- **Foreign key naming patterns**: Fields ending in "Id", "ID", or "id"
- **Schema name matching**: Matches field prefixes with other file names
- **Plural/singular matching**: Handles variations like "userId" → "user" or "categoriesId" → "category"

### 2. Dependency Ordering

Files are processed in the correct order:

- Referenced tables (parent tables) are created first
- Dependent tables (child tables with foreign keys) are created after

### 3. Field Type Detection

Each field automatically gets the appropriate type:

- **reference**: For detected foreign key relationships (links to other containers)
- **int/float/bool**: For numeric and boolean data
- **date**: For date values
- **string**: For text data
- **email/url/uuid/ip**: For special string types

## Request Format

### Using cURL

```bash
curl -X POST "http://localhost:4000/api/v1/myTenant/myProject/excel/upload-multiple" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "files=@users.xlsx" \
  -F "files=@orders.xlsx" \
  -F "files=@products.xlsx"
```

### Using Postman

1. Set method to `POST`
2. URL: `http://localhost:4000/api/v1/:tenantSlug/:projectSlug/excel/upload-multiple`
3. Headers: `Authorization: Bearer YOUR_TOKEN`
4. Body → form-data:
   - Key: `files` (set type to File, allow multiple)
   - Value: Select multiple Excel files

### Using JavaScript (Fetch API)

```javascript
const formData = new FormData();
formData.append("files", usersFile);
formData.append("files", ordersFile);
formData.append("files", productsFile);

const response = await fetch(
  "http://localhost:4000/api/v1/myTenant/myProject/excel/upload-multiple",
  {
    method: "POST",
    headers: {
      Authorization: "Bearer YOUR_TOKEN",
    },
    body: formData,
  },
);

const result = await response.json();
console.log(result);
```

## Example Scenario

### Input Files

#### users.xlsx

| id  | name     | email            | role  |
| --- | -------- | ---------------- | ----- |
| 1   | John Doe | john@example.com | admin |
| 2   | Jane Doe | jane@example.com | user  |

#### orders.xlsx

| orderId | userId | productId | quantity | orderDate  |
| ------- | ------ | --------- | -------- | ---------- |
| 101     | 1      | 501       | 2        | 2024-01-15 |
| 102     | 2      | 502       | 1        | 2024-01-16 |

#### products.xlsx

| productId | productName | price | stock |
| --------- | ----------- | ----- | ----- |
| 501       | Laptop      | 999   | 50    |
| 502       | Mouse       | 25    | 200   |

### Detected Relationships

```json
{
  "relationships": [
    {
      "sourceSchema": "orders",
      "sourceField": "userId",
      "targetSchema": "users",
      "confidence": "high"
    },
    {
      "sourceSchema": "orders",
      "sourceField": "productId",
      "targetSchema": "products",
      "confidence": "high"
    }
  ]
}
```

### Creation Order

1. `users` (no dependencies)
2. `products` (no dependencies)
3. `orders` (depends on users and products)

### Resulting Schema

#### users Container

```json
{
  "fields": [
    { "name": "id", "type": "int" },
    { "name": "name", "type": "string" },
    { "name": "email", "type": "string" },
    { "name": "role", "type": "string" }
  ]
}
```

#### products Container

```json
{
  "fields": [
    { "name": "productId", "type": "int" },
    { "name": "productName", "type": "string" },
    { "name": "price", "type": "int" },
    { "name": "stock", "type": "int" }
  ]
}
```

#### orders Container

```json
{
  "fields": [
    { "name": "orderId", "type": "int" },
    { "name": "userId", "type": "reference", "objectSchemaName": "users" },
    {
      "name": "productId",
      "type": "reference",
      "objectSchemaName": "products"
    },
    { "name": "quantity", "type": "int" },
    { "name": "orderDate", "type": "date" }
  ]
}
```

## Response Format

### Success Response (201 Created)

```json
{
  "status": 201,
  "message": "Multiple Excel files successfully imported with relationships",
  "data": {
    "totalFiles": 3,
    "containers": [
      {
        "schemaName": "users",
        "containerId": "65b1234567890abcdef12345",
        "rowsInserted": 2,
        "fields": [...]
      },
      {
        "schemaName": "products",
        "containerId": "65b1234567890abcdef12346",
        "rowsInserted": 2,
        "fields": [...]
      },
      {
        "schemaName": "orders",
        "containerId": "65b1234567890abcdef12347",
        "rowsInserted": 2,
        "fields": [...]
      }
    ],
    "relationships": [
      {
        "sourceSchema": "orders",
        "sourceField": "userId",
        "targetSchema": "users",
        "confidence": "high"
      },
      {
        "sourceSchema": "orders",
        "sourceField": "productId",
        "targetSchema": "products",
        "confidence": "high"
      }
    ]
  }
}
```

### Error Responses

#### 400 Bad Request

```json
{
  "status": 400,
  "message": "At least one Excel file is required",
  "data": null
}
```

#### 500 Internal Server Error

```json
{
  "status": 500,
  "message": "Failed to create container users: ...",
  "data": null
}
```

## Best Practices

### 1. File Naming

- Use clear, descriptive names: `users.xlsx`, `orders.xlsx`, `products.xlsx`
- Avoid special characters and spaces
- The filename becomes the schema/container name

### 2. Column Naming for Relationships

For automatic detection to work:

- **Foreign keys should end with "Id"**: `userId`, `productId`, `categoryId`
- Match the referenced table name: `userId` references `user` or `users`
- Use camelCase or lowercase consistently

### 3. Data Quality

- Ensure headers are in the first row
- Include at least one data row in each file
- Use consistent data types in each column
- Empty cells are okay, but avoid mixing data types

### 4. Upload Order

- Files can be uploaded in any order
- The system automatically determines the correct creation sequence
- Parent tables are always created before child tables

### 5. Reference Data

- For initial upload, you can use placeholder IDs
- After upload, you'll need to update references to actual MongoDB ObjectIDs
- Or use the original ID fields for lookups

## Advanced Features

### Confidence Levels

The system assigns confidence levels to detected relationships:

- **high**: Exact or near-exact schema name match with "Id" suffix
- **medium**: Requires plural/singular conversion or partial match
- **low**: Weak pattern match (not currently used)

### Handling Complex Names

The system handles various naming patterns:

```
userId → user, users
productId → product, products
categoryId → category, categories
orderId → order, orders
customerId → customer, customers
```

### Manual Relationship Override

If automatic detection misses a relationship, you can:

1. Upload files individually using `/upload`
2. Manually edit the container schema via the API
3. Add `objectSchemaName` to the field definition

## Limitations

1. **Single-level relationships only**: Currently detects direct foreign key relationships
2. **ObjectID conversion**: Original IDs in Excel need manual mapping to MongoDB ObjectIDs
3. **No constraint enforcement**: References are logical only, no database constraints
4. **First sheet only**: Only the first sheet of each Excel file is processed

## Future Enhancements

- Support for many-to-many relationships
- Automatic ID mapping and conversion
- Multi-sheet Excel support
- Relationship validation during data insert
- Visual relationship diagram generation
- Support for composite foreign keys

## Troubleshooting

### Relationships Not Detected

**Problem**: Foreign keys not recognized

**Solutions**:

- Ensure field names end with "Id", "ID", or "id"
- Match field prefix with target schema name
- Example: For a `users` table, use `userId`, not `user_id` or `userID`

### Wrong Creation Order

**Problem**: Child table created before parent

**Solution**:

- Check the response's `relationships` array
- Verify foreign key naming follows conventions
- Use singular/plural variations correctly

### Type Detection Issues

**Problem**: Wrong field types assigned

**Solution**:

- Ensure consistent data in each column
- For integers: avoid decimals in the data
- For dates: use recognized date formats (YYYY-MM-DD, MM/DD/YYYY, etc.)
- Edit the container schema after upload if needed

### Large File Performance

**Problem**: Timeout with many files or large datasets

**Solutions**:

- Upload files in smaller batches
- Increase server timeout settings
- Use the single-file `/upload` endpoint for very large files
- Consider database indexing after upload

## Testing the Feature

### Test Data Setup

Create three simple Excel files to test:

**test_users.xlsx**:

```
name,email
Alice,alice@test.com
Bob,bob@test.com
```

**test_orders.xlsx**:

```
orderId,userId,amount
1,1,100
2,2,200
```

Upload both files and verify the `userId` in orders is detected as a reference to `users`.
