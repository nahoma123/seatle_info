# Frontend API: Search Listings Guide

This guide provides details on how to use the `GET /api/v1/listings` endpoint to search and filter listings. This endpoint is powered by Elasticsearch for fast and relevant results.

## Endpoint Overview

*   **URL:** `GET /api/v1/listings`
*   **Method:** `GET`
*   **Authentication:** Public.
    *   Contact information (email, phone) within listings may be masked or omitted if the request is not authenticated with a valid Bearer token.
*   **Description:** Retrieves a paginated list of listings based on various search and filter criteria.

## Query Parameters (All Optional)

All query parameters are optional. If no parameters are provided, the endpoint will return a paginated list of default active, admin-approved, and non-expired listings.

| Parameter         | Type          | Description                                                                                                                                                              | Example                                        |
| ----------------- | ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------- |
| `q`               | string        | General search term. Searches across `title`, `description`, `contact_name`, and `address_line1`.                                                                          | `q=monthly%20apartment`                        |
| `category_id`     | string (UUID) | Filter by a specific main category ID.                                                                                                                                   | `category_id=...`                              |
| `sub_category_id` | string (UUID) | Filter by a specific sub-category ID.                                                                                                                                    | `sub_category_id=...`                          |
| `user_id`         | string (UUID) | Filter listings created by a specific user.                                                                                                                              | `user_id=...`                                  |
| `status`          | string        | Filter by listing status. Allowed values: `active`, `pending_approval`, `expired`, `rejected`, `admin_removed`. **Default behavior (if `status` and `include_expired` are not provided):** Shows `active` listings that are `is_admin_approved: true` and have `expires_at > now`. | `status=active`                                |
| `include_expired` | boolean       | If `true` and `status` is not provided, the search may include listings of any status (respecting the `is_admin_approved: true` default) and removes the `expires_at > now` filter. If `status` *is* provided, `include_expired` is generally ignored in favor of the explicit status filter (though expired listings can still be fetched if `status=expired`). | `include_expired=true`                         |
| `lat`             | number (float64) | Latitude for location-based search. Requires `lon` to be specified.                                                                                                      | `lat=47.6062`                                  |
| `lon`             | number (float64) | Longitude for location-based search. Requires `lat` to be specified.                                                                                                     | `lon=-122.3321`                                |
| `max_distance_km` | number (float64) | Maximum distance in kilometers for location-based search. Requires `lat` and `lon`.                                                                                      | `max_distance_km=5`                            |
| `sort_by`         | string        | Field to sort results by. Allowed values: `distance` (requires `lat`, `lon`), `created_at`, `expires_at`, `title`. **Default:** `distance` if `lat` & `lon` are provided; `_score` (relevance) if `q` is provided; otherwise `created_at`. | `sort_by=created_at`                           |
| `sort_order`      | string        | Sort order. Allowed values: `asc` (ascending), `desc` (descending). **Default:** `asc` for `distance`; `desc` for `created_at` and `_score`.                                | `sort_order=desc`                              |
| `page`            | integer       | Page number for pagination.                                                                                                                                              | `page=2` (Default: 1)                          |
| `page_size`       | integer       | Number of items per page.                                                                                                                                                | `page_size=20` (Default: 10)                   |

**Note on `is_admin_approved`:**
Unless a specific `status` query parameter implies otherwise (e.g. `status=pending_approval`), results are generally filtered to include only listings where `is_admin_approved` is `true`.

## Example Queries

1.  **Search for "delicious pizza" near downtown Seattle (latitude 47.6062, longitude -122.3321) within a 5km radius, sorted by distance:**
    ```
    GET /api/v1/listings?q=delicious%20pizza&lat=47.6062&lon=-122.3321&max_distance_km=5&sort_by=distance&sort_order=asc
    ```

2.  **Get active "Housing" listings (assuming category ID for Housing is `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`), newest first, page 2, 15 items per page:**
    ```
    GET /api/v1/listings?category_id=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx&status=active&sort_by=created_at&sort_order=desc&page=2&page_size=15
    ```

## Successful Response (`200 OK`)

The API returns a JSON object with the following structure:

```json
{
  "success": true,
  "message": "Listings retrieved successfully.",
  "data": [
    // Array of ListingResponse objects
  ],
  "pagination": {
    "current_page": 1,
    "page_size": 10,
    "total_items": 57,
    "total_pages": 6
  }
}
```

### `ListingResponse` Object Details

Each object in the `data` array is a `ListingResponse`. The fields are based on the `Listing` model, with some processing for frontend display.

```json
{
  "id": "uuid-string-for-listing",
  "user_id": "uuid-string-for-user",
  "user": { // Simplified User object
    "id": "uuid-string-for-user",
    "username": "listing_creator_username"
  },
  "category_id": "uuid-string-for-category",
  "category": { // Category object
    "id": "uuid-string-for-category",
    "name": "Category Name",
    "slug": "category-slug"
  },
  "sub_category_id": "uuid-string-for-sub-category", // Can be null
  "sub_category": { // SubCategory object, can be null
    "id": "uuid-string-for-sub-category",
    "name": "SubCategory Name",
    "slug": "sub-category-slug"
  },
  "title": "Example Listing Title",
  "description": "Detailed description of the listing.",
  "status": "active", // e.g., active, pending_approval
  "contact_name": "John Doe", // May be present
  "contact_email": "user@example.com", // Masked or omitted if not authenticated or not owner
  "contact_phone": "555-1234",      // Masked or omitted
  "address_line1": "123 Main St",
  "address_line2": "Apt 4B", // Can be null
  "city": "Seattle",
  "state": "WA",
  "zip_code": "98101",
  "latitude": 47.6062,  // Can be null
  "longitude": -122.3321, // Can be null
  "distance_km": 1.234, // Only present when sorting by distance or geo-searching
  "expires_at": "2024-12-31T23:59:59Z",
  "created_at": "2024-01-15T10:00:00Z",
  "updated_at": "2024-01-16T11:30:00Z",
  "is_admin_approved": true,
  "admin_notes": null, // Typically null for public users
  // Category-specific details (only one of these will be present based on category)
  "event_details": {
    "event_date": "2024-08-15T00:00:00Z",
    "event_time": "14:00:00",
    "organizer_name": "Community Group",
    "venue_name": "Town Hall"
  },
  "housing_details": {
    "property_type": "apartment_for_rent", // e.g., apartment_for_rent, house_for_sale
    "rent_details": "Includes utilities", // If for rent
    "sale_price": 500000.00 // If for sale
  },
  "babysitting_details": {
    "languages_spoken": ["English", "Spanish"]
  }
}
```
**Note:** The exact fields within `event_details`, `housing_details`, and `babysitting_details` will depend on the listing's category. Only one of these detail blocks will be present. Contact fields (`contact_email`, `contact_phone`) might be omitted or masked based on authentication status and user permissions. The `distance_km` field is only populated if the search involved a geo-location query and sorting by distance.

## Error Responses

*   **`400 Bad Request`**: Indicates invalid query parameters (e.g., malformed UUID, invalid enum value for `status` or `sort_by`).
    ```json
    {
      "success": false,
      "message": "Validation Error",
      "errors": [
        {
          "field": "max_distance_km",
          "message": "max_distance_km must be a positive number when lat and lon are provided"
        }
      ]
    }
    ```
*   **`500 Internal Server Error`**: Indicates an unexpected error on the server while processing the request.
    ```json
    {
      "success": false,
      "message": "An unexpected error occurred.",
      "error_code": "INTERNAL_SERVER_ERROR" // Example, actual error_code might vary
    }
    ```

---

This guide should help frontend developers understand and integrate with the listing search API.
