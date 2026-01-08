# Excel Upload Feature

## Overview

This feature allows you to upload Excel files and automatically create containers (database tables) based on the Excel data structure.

## How It Works

### 1. Upload an Excel File

The system will:

- Read the first sheet of your Excel file
- Use the first row as column headers
- Analyze the data to determine field types automatically
- Create a container with the same name as your table
- Import all the data rows into the new collection

### 2. Field Type Detection

The system automatically detects field types based on the data:

- **Email**: If the value contains "@" and "."
- **Date**: If the value matches common date formats (YYYY-MM-DD, MM/DD/YYYY, etc.)
- **Number**: If all values in the column are numeric
- **Text**: Default for everything else

### 3. Field Name Sanitization

Column headers are converted to camelCase field names:

- "First Name" → "firstName"
- "Email Address" → "emailAddress"
- "Product ID" → "productId"

## API Endpoint

### Upload Excel

**POST** `/api/v1/:tenantSlug/:projectSlug/excel/upload`

**Headers:**

- `Authorization: Bearer <your-jwt-token>`

**Form Data:**

- `file` (required): The Excel file (.xlsx)
- `tableName` (required): Name for the container/table
- `pageId` (optional): ID of the page to link this container to

**Example using cURL:**

```bash
curl -X POST \
  http://localhost:3002/api/v1/acme/deneme/excel/upload \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -F "file=@/path/to/your/file.xlsx" \
  -F "tableName=customers" \
  -F "pageId=60f7b3b3b3b3b3b3b3b3b3b3"
```

**Example using JavaScript (Frontend):**

```javascript
const formData = new FormData();
formData.append("file", fileInput.files[0]);
formData.append("tableName", "customers");
formData.append("pageId", "your-page-id"); // Optional

const response = await fetch(
  "http://localhost:3002/api/v1/acme/deneme/excel/upload",
  {
    method: "POST",
    headers: {
      Authorization: `Bearer ${accessToken}`,
    },
    body: formData,
  }
);

const result = await response.json();
console.log(result);
```

## Response Format

### Success Response (201 Created)

```json
{
  "status": 201,
  "message": "Excel data successfully imported",
  "data": {
    "containerId": "60f7b3b3b3b3b3b3b3b3b3b3",
    "tableName": "customers",
    "rowsInserted": 150,
    "fields": [
      {
        "name": "firstName",
        "label": "First Name",
        "type": "text",
        "required": false,
        "unique": false
      },
      {
        "name": "email",
        "label": "Email",
        "type": "email",
        "required": false,
        "unique": false
      }
    ]
  }
}
```

### Error Response (400 Bad Request)

```json
{
  "status": 400,
  "message": "Table name is required"
}
```

## Excel File Requirements

1. **File Format**: .xlsx (Excel 2007+)
2. **Structure**:
   - First row MUST be headers
   - At least one data row is required
3. **Headers**: Should be descriptive (will be used as field labels)
4. **Data**: Clean data helps with better type detection

## Example Excel File Structure

| First Name | Last Name | Email            | Age | Registration Date |
| ---------- | --------- | ---------------- | --- | ----------------- |
| John       | Doe       | john@example.com | 30  | 2024-01-15        |
| Jane       | Smith     | jane@example.com | 25  | 2024-01-16        |

This will create a container with fields:

- `firstName` (text)
- `lastName` (text)
- `email` (email)
- `age` (number)
- `registrationDate` (date)

## Linking to Pages

If you provide a `pageId` when uploading:

- The container will be linked to that page
- The container's `pageId` field will be set
- You can use this to organize containers under different tabs/pages in your UI

## Future Enhancements

Planned improvements:

- Support for multiple sheets
- Custom field type mapping
- Validation rules from Excel
- Update existing containers
- Bulk data updates
- Support for relationships between tables
