# AutoTable Dynamic Swagger Documentation

## Overview

AutoTable now includes a dynamic Swagger/OpenAPI documentation system that automatically generates API documentation for all your schemas/containers. This documentation is updated in real-time as you add, modify, or remove containers.

## Features

✅ **Dynamic Schema Generation**: Automatically generates OpenAPI schemas for all containers  
✅ **Complete CRUD Operations**: Documents all endpoints (GET, POST, PATCH, DELETE)  
✅ **Advanced Operations**: Includes pagination, search, filtering, and bulk operations  
✅ **Interactive UI**: Full Swagger UI with try-it-out functionality  
✅ **Real-time Updates**: Documentation updates automatically when containers change  
✅ **Type Safety**: Proper typing for all field types including ObjectID references

## Endpoints

### Swagger UI

- **URL**: `GET /api/swagger`
- **Description**: Interactive Swagger UI interface
- **Usage**: Open this in your browser to explore and test your APIs

### Swagger JSON Specification

- **URL**: `GET /api/swagger.json`
- **Description**: Raw OpenAPI 3.0 JSON specification
- **Usage**: Use this URL in tools like Postman, Insomnia, or other API clients

### Schema List

- **URL**: `GET /api/schemas`
- **Description**: Lists all available schemas with metadata
- **Response**:

```json
{
  "schemas": [
    {
      "name": "users",
      "fieldCount": 5,
      "hasRoutes": true,
      "hasPipelines": true,
      "fields": [...]
    }
  ],
  "total": 3
}
```

## Supported Field Types

The swagger generator automatically maps your field types:

| AutoTable Type    | OpenAPI Type         | Description                |
| ----------------- | -------------------- | -------------------------- |
| `string`          | `string`             | Text field                 |
| `number`          | `number`             | Numeric field              |
| `boolean`         | `boolean`            | True/false field           |
| `date`            | `string` (date-time) | ISO date field             |
| `objectId`        | `string` (pattern)   | MongoDB ObjectID reference |
| `autoIncrementId` | `integer`            | Auto-incrementing ID       |
| `image`           | `string` (uri)       | Image URL                  |
| `password`        | `string` (password)  | Password field             |

## Generated Operations

For each schema, the following operations are automatically documented:

### Basic CRUD

- `GET /api/v1/dynamic/` - Get all items
- `POST /api/v1/dynamic/` - Create new item
- `GET /api/v1/dynamic/{id}` - Get single item
- `PATCH /api/v1/dynamic/{id}` - Update item
- `DELETE /api/v1/dynamic/{id}` - Delete item

### Advanced Operations

- `GET /api/v1/dynamic/page` - Paginated results with optional search
- `GET /api/v1/dynamic/search` - Search items
- `GET /api/v1/dynamic/filter` - Filter items
- `POST /api/v1/dynamic/multiple` - Create multiple items
- `PATCH /api/v1/dynamic/multiple` - Update multiple items
- `DELETE /api/v1/dynamic/multiple` - Delete multiple items

### Pipeline & Custom Operations

- `GET /api/v1/dynamic/pipeline` - Execute pipelines
- `GET /api/v1/dynamic/execute` - Execute dynamic functions
- `GET /api/v1/dynamic/api` - Execute dynamic APIs

## Usage Examples

### 1. Access Swagger UI

```
http://localhost:3000/api/swagger
```

### 2. Get API Specification

```bash
curl http://localhost:3000/api/swagger.json
```

### 3. List Available Schemas

```bash
curl http://localhost:3000/api/schemas
```

### 4. Use with Postman

1. In Postman, go to **Import**
2. Choose **Link**
3. Enter: `http://localhost:3000/api/swagger.json`
4. Import the collection

## Security Considerations

- The swagger endpoints are currently public
- Consider adding authentication if needed for production
- The documentation reflects your actual database schema

## Customization

The swagger generation can be customized by modifying:

- `/controllers/swaggerController.go` - Core generation logic
- Field descriptions and validation rules
- Security schemes and authentication

## Troubleshooting

### Documentation Not Updating

- Restart your server to refresh container cache
- Check that your containers are properly saved in the database

### Missing Endpoints

- Ensure your containers have the proper field definitions
- Check the console for any error messages during generation

### UI Not Loading

- Verify the swagger UI assets are accessible
- Check browser console for JavaScript errors

## Integration with Other Tools

The generated OpenAPI specification works with:

- **Postman**: Import collections
- **Insomnia**: API testing
- **OpenAPI Generator**: Generate client SDKs
- **API Gateway Tools**: Import into Azure API Management, AWS API Gateway
- **Documentation Sites**: Generate static docs with tools like Redoc

## Benefits

🚀 **Faster Development**: Instantly test APIs without writing custom clients  
📚 **Better Documentation**: Always up-to-date API docs  
🔄 **Team Collaboration**: Share consistent API specifications  
🧪 **Easy Testing**: Built-in testing interface  
⚡ **Rapid Prototyping**: Quickly validate API designs

---

**Note**: The swagger documentation is generated dynamically based on your container models. Any changes to your containers will be reflected in the documentation after the server restarts or cache refresh.
