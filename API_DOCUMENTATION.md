# API Documentation - Seattle Info Backend

## Notes on Documentation

*   **Auth: Bearer Token (Firebase ID Token)**: Indicates that the endpoint requires authentication. The client must include a Firebase ID Token (obtained from Firebase upon successful sign-in) in the `Authorization` header with the `Bearer` scheme. Example: `Authorization: Bearer <FIREBASE_ID_TOKEN>`.
*   **Auth: Admin (Bearer Token) (Firebase ID Token)**: Indicates that the endpoint requires authentication and that the authenticated user must have an "admin" role. The token is a Firebase ID Token from an admin user.
*   **Public**: Indicates that the endpoint does not require authentication.
*   **Request Body Validation**: Most `POST` and `PUT` endpoints validate the request body. If validation fails, a `422 Unprocessable Entity` error is returned with details about the validation failures.
*   **Response Bodies**: Example response bodies are illustrative and may omit some fields for brevity or include sample data. Refer to the field descriptions for complete details.
*   **IDs**: All IDs (e.g., user ID, category ID, listing ID) are UUIDs.
*   **Timestamps**: All timestamps (e.g., `created_at`, `updated_at`) are in UTC and formatted according to RFC3339 (e.g., `2023-10-26T10:00:00Z`).

---

## Module: Health Check

Provides an endpoint to check the health status of the API.

### `GET /api/v1/health`

*   **Description**: Checks the operational status of the API.
*   **Auth**: Public
*   **Request Body**: None
*   **Response**: `200 OK`
    ```json
    {
        "status": "UP",
        "message": "Seattle Info API is healthy!"
    }
    ```

============================

## Module: User Authentication (Auth)

Handles user authentication using Firebase. Client applications are responsible for user sign-up and sign-in using Firebase SDKs (e.g., FirebaseUI for Web/Android/iOS, or direct SDK integration). Upon successful sign-in, Firebase provides a Firebase ID Token to the client. This token must be sent by the client in the `Authorization` header for all authenticated API requests.

**Authorization Header Format:**
`Authorization: Bearer <FIREBASE_ID_TOKEN>`

Backend endpoints for direct login, registration, token refresh, or specific OAuth provider interactions (e.g., `/google/login`, `/apple/login`) are no longer provided as these processes are now managed by Firebase on the client-side.

### `GET /api/v1/auth/me`

*   **Description**: Retrieves the profile of the currently authenticated user based on the provided Firebase ID Token. This is the primary way to verify a token and get user details.
*   **Auth**: Bearer Token (Firebase ID Token)
*   **Request Body**: None
*   **Headers**:
    *   `Authorization: Bearer <FIREBASE_ID_TOKEN>` (Required)
*   **Response**: `200 OK`
    ```json
    {
        "id": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
        "email": "user@example.com",
        "first_name": "John",
        "last_name": "Doe",
        "profile_picture_url": "http://example.com/profile.jpg",
        "auth_provider": "firebase", // Indicates user authenticated via Firebase
        "is_email_verified": true,
        "role": "user", // or "admin"
        "is_first_post_approved": true,
        "created_at": "2023-01-15T10:00:00Z",
        "updated_at": "2023-01-16T11:30:00Z",
        "last_login_at": "2023-10-26T12:00:00Z"
    }
    ```
*   **Error Responses**:
    *   `401 Unauthorized`: If the token is invalid, expired, or not provided.
        ```json
        {
            "code": "UNAUTHORIZED",
            "message": "Authentication is required and has failed or has not yet been provided.",
            "details": "Invalid or expired token: firebase id token has expired" // Example detail
        }
        ```
    *   `500 Internal Server Error`: If there's an issue fetching the user from the database after successful token verification.

============================

## Module: Users

Manages user profiles. User registration is now handled by the client application using Firebase SDKs. The backend expects a Firebase ID Token for authenticated user actions.

### `GET /api/v1/users/{id}`

*   **Description**: Retrieves the public profile of a specific user by their ID. Access might be restricted based on privacy settings or requester's role (e.g., only admins or the user themselves can view detailed profiles).
*   **Auth**: Bearer Token (Firebase ID Token) - Required if the endpoint is not public.
    *   *Note*: This endpoint might be public for basic profile information or restricted. The example assumes authenticated access for a full profile. If public, the `Authorization` header is optional.
*   **Request Body**: None
*   **Path Parameters**:
    *   `id` (UUID, required): The ID of the user to retrieve.
*   **Headers**:
    *   `Authorization: Bearer <FIREBASE_ID_TOKEN>` (Required if not a public endpoint or for full details)
*   **Response**: `200 OK`
    ```json
    {
        "id": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
        "first_name": "Jane",
        "last_name": "Doe",
        "profile_picture_url": "http://example.com/jane_profile.jpg",
        "auth_provider": "firebase", // User was created/linked via Firebase
        // Other fields like 'email', 'role', 'is_email_verified' might be restricted
        // depending on who is making the request (e.g., admin or self vs. other user).
        // For this example, we assume a restricted view for non-admin/non-self requests.
        // If it's the user themselves or an admin, more fields would be present like in /auth/me.
        "created_at": "2023-02-10T09:00:00Z"
    }
    ```
*   **Error Responses**:
    *   `400 Bad Request`: If the `id` path parameter is not a valid UUID.
        ```json
        {
            "code": "BAD_REQUEST",
            "message": "The request is invalid.",
            "details": "Invalid user ID format."
        }
        ```
    *   `401 Unauthorized`: If authentication is required and the token is invalid or missing.
    *   `403 Forbidden`: If the authenticated user does not have permission to view the requested profile (e.g., trying to access private details of another user without admin rights).
    *   `404 Not Found`: If no user with the specified ID exists.
        ```json
        {
            "code": "NOT_FOUND",
            "message": "The requested resource could not be found.",
            "details": "User not found with this ID."
        }
        ```
    *   `500 Internal Server Error`.

---
## Module: Categories
Manages categories for listings.

### `GET /api/v1/categories`
*   **Description**: Retrieves a list of all available categories.
*   **Auth**: Public
*   **Query Parameters**:
    *   `page` (int, optional, default: 1): The page number for pagination.
    *   `page_size` (int, optional, default: 10): The number of categories per page.
*   **Response**: `200 OK`
    ```json
    {
        "data": [
            {
                "id": "c1d2e3f4-a5b6-7890-1234-567890abcdef",
                "name": "Electronics",
                "slug": "electronics",
                "description": "Gadgets, computers, and more.",
                "created_at": "2023-01-01T10:00:00Z",
                "updated_at": "2023-01-01T11:00:00Z"
            },
            {
                "id": "b1c2d3e4-f5a6-b789-0123-456789abcdef",
                "name": "Furniture",
                "slug": "furniture",
                "description": "Home and office furniture.",
                "created_at": "2023-01-02T12:00:00Z",
                "updated_at": "2023-01-02T13:00:00Z"
            }
        ],
        "pagination": {
            "current_page": 1,
            "page_size": 10,
            "total_records": 25,
            "total_pages": 3
        }
    }
    ```

### `POST /api/v1/categories`
*   **Description**: Creates a new category.
*   **Auth**: Admin (Bearer Token)
*   **Request Body**:
    ```json
    {
        "name": "Books",
        "description": "Fiction, non-fiction, textbooks."
    }
    ```
*   **Response**: `201 Created`
    ```json
    {
        "id": "d1e2f3a4-b5c6-d789-e012-3456789abcde",
        "name": "Books",
        "slug": "books",
        "description": "Fiction, non-fiction, textbooks.",
        "created_at": "2023-10-27T14:00:00Z",
        "updated_at": "2023-10-27T14:00:00Z"
    }
    ```
*   **Error Responses**: `400`, `401`, `403`, `422`, `500`

---
## Module: Listings
Manages listings posted by users.

### `GET /api/v1/listings`
*   **Description**: Retrieves a list of all active listings, possibly filtered by various criteria.
*   **Auth**: Public
*   **Query Parameters**:
    *   `page` (int, optional, default: 1): Page number.
    *   `page_size` (int, optional, default: 10): Number of listings per page.
    *   `category_id` (UUID, optional): Filter by category ID.
    *   `user_id` (UUID, optional): Filter by user ID (who posted the listing).
    *   `status` (string, optional): Filter by listing status (e.g., "active", "expired").
    *   `search_term` (string, optional): Search by keyword in title/description.
    *   `latitude` (float, optional): Latitude for location-based search.
    *   `longitude` (float, optional): Longitude for location-based search.
    *   `radius_km` (float, optional): Radius in kilometers for location-based search (requires latitude & longitude).
*   **Response**: `200 OK`
    ```json
    {
        "data": [
            {
                "id": "l1m2n3o4-p5q6-r789-s012-t3456789uvwx",
                "title": "Vintage Armchair",
                "description": "Comfortable vintage armchair, good condition.",
                "category_id": "b1c2d3e4-f5a6-b789-0123-456789abcdef",
                "user_id": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
                "price": 75.00,
                "status": "active",
                "latitude": 47.6062,
                "longitude": -122.3321,
                "created_at": "2023-10-20T09:00:00Z",
                "updated_at": "2023-10-21T10:00:00Z",
                "expires_at": "2023-11-20T09:00:00Z"
            }
            // ... more listings
        ],
        "pagination": {
            "current_page": 1,
            "page_size": 10,
            "total_records": 50,
            "total_pages": 5
        }
    }
    ```

### `POST /api/v1/listings`
*   **Description**: Creates a new listing.
*   **Auth**: Bearer Token (Firebase ID Token)
*   **Request Body**:
    ```json
    {
        "title": "Used Bicycle",
        "description": "Men's mountain bike, 21 speeds, needs minor repairs.",
        "category_id": "c1d2e3f4-a5b6-7890-1234-567890abcdef", // Example: Electronics, adjust as needed
        "price": 120.50,
        "latitude": 47.6097,  // Optional
        "longitude": -122.3331 // Optional
    }
    ```
*   **Response**: `201 Created`
    ```json
    {
        "id": "k1j2h3g4-f5e6-d789-c012-b3456789axyz",
        "title": "Used Bicycle",
        "description": "Men's mountain bike, 21 speeds, needs minor repairs.",
        "category_id": "c1d2e3f4-a5b6-7890-1234-567890abcdef",
        "user_id": "current_authenticated_user_id", // Set by backend
        "price": 120.50,
        "status": "active", // Default status
        "latitude": 47.6097,
        "longitude": -122.3331,
        "created_at": "2023-10-27T15:00:00Z",
        "updated_at": "2023-10-27T15:00:00Z",
        "expires_at": "2023-11-06T15:00:00Z" // Calculated by backend
    }
    ```
*   **Error Responses**: `400`, `401`, `422`, `500`
```

### `GET /api/v1/listings/my-listings`
*   **Method & Path:** `GET /api/v1/listings/my-listings`
*   **Description:** Retrieves a list of all listings created by the authenticated user.
*   **Authentication:** Required (Bearer Token - Firebase ID Token).
*   **Query Parameters:**
    *   `page` (int, optional, default: 1): Page number for pagination.
    *   `page_size` (int, optional, default: 10): Number of items per page.
    *   `status` (string, optional): Filter by listing status (e.g., "active", "pending_approval", "expired", "rejected", "admin_removed").
    *   `category_slug` (string, optional): Filter by category slug (e.g., "events", "housing", "baby-sitting").
*   **Successful Response (200 OK):**
    *   The response is a paginated list of listing objects. Each listing object includes full details, including category information, sub-category information (if applicable), and the relevant category-specific details block (e.g., `event_details`, `housing_details`).
    ```json
    {
        "message": "Successfully retrieved your listings.",
        "data": [
            {
                "id": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
                "user_id": "u1v2w3x4-y5z6-7890-1234-567890qrstuv",
                "title": "Community Summer Fest",
                "description": "Annual summer festival with games, food, and music.",
                "status": "active",
                "contact_name": "Jane Doe",
                "contact_email": "jane.doe@example.com",
                "contact_phone": "555-0101",
                "address_line1": "123 Main St",
                "address_line2": "Suite 100",
                "city": "Seattle",
                "state": "WA",
                "zip_code": "98101",
                "latitude": 47.6062,
                "longitude": -122.3321,
                "expires_at": "2024-09-01T00:00:00Z",
                "is_admin_approved": true,
                "created_at": "2024-03-15T10:00:00Z",
                "updated_at": "2024-03-15T11:30:00Z",
                "user": { // User who posted the listing
                    "id": "u1v2w3x4-y5z6-7890-1234-567890qrstuv",
                    "first_name": "John",
                    "last_name": "Doe",
                    "profile_picture_url": null
                },
                "category": { // Main category details
                    "id": "cat_events_uuid",
                    "name": "Events",
                    "slug": "events",
                    "description": "Community events and gatherings."
                },
                "sub_category": null, // or populated sub-category object
                "event_details": { // Category-specific details
                    "listing_id": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
                    "event_date": "2024-07-20T00:00:00Z",
                    "event_time": "10:00:00",
                    "organizer_name": "City Events Committee",
                    "venue_name": "Downtown Park"
                },
                "housing_details": null, // Only one detail type will be populated
                "babysitting_details": null
            }
            // ... other listings ...
        ],
        "pagination": {
            "current_page": 1,
            "page_size": 10,
            "total_items": 1,
            "total_pages": 1
        }
    }
    ```
*   **Error Responses:**
    *   `401 Unauthorized`: If the user is not authenticated.
    *   `500 Internal Server Error`: For unexpected server issues.

### `PUT /api/v1/listings/{listing_id}`
*   **Method & Path:** `PUT /api/v1/listings/{listing_id}`
*   **Description:** Allows an authenticated user to update the details of a listing they own. Fields not provided in the request body will generally remain unchanged (partial update).
*   **Authentication:** Required (Bearer Token - Firebase ID Token).
*   **URL Parameters:**
    *   `listing_id` (UUID, required): The ID of the listing to update.
*   **Request Body:**
    *   The request body should be a JSON object containing the fields to update. This can include top-level listing fields and/or category-specific detail fields.
    *   The category of a listing cannot be changed.
    *   `status` and `is_admin_approved` fields are not modifiable via this endpoint.
    *   **Example for an Event Listing Update:**
        ```json
        {
            "title": "Updated Community Summer Fest",
            "description": "Updated details: Now with more food trucks and live bands!",
            "contact_phone": "555-0199",
            "event_details": { // Key based on category (e.g., housing_details, babysitting_details)
                "event_date": "2024-07-21",   // Date string "YYYY-MM-DD"
                "event_time": "11:00:00",     // Time string "HH:MM:SS"
                "venue_name": "Central City Park"
            }
        }
        ```
    *   **Example for a Housing Listing Update (partial - only rent details):**
        ```json
        {
            "housing_details": {
                "property_type": "for_rent", // Must match existing or be a valid update based on rules
                "rent_details": "$2200/month, utilities included"
            }
        }
        ```
*   **Successful Response (200 OK):**
    *   Returns the fully updated listing object, similar to the `GET /api/v1/listings/{listing_id}` response structure.
    ```json
    {
        "message": "Listing updated successfully.",
        "data": {
            "id": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
            "user_id": "u1v2w3x4-y5z6-7890-1234-567890qrstuv",
            "category_id": "cat_events_uuid",
            "title": "Updated Community Summer Fest",
            "description": "Updated details: Now with more food trucks and live bands!",
            "contact_phone": "555-0199",
            // ... other fields ...
            "updated_at": "2024-03-15T14:00:00Z", // Note: timestamp reflects update time
            "user": { /* ... user details ... */ },
            "category": { /* ... category details for "events" ... */ },
            "sub_category": null,
            "event_details": {
                "listing_id": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
                "event_date": "2024-07-21T00:00:00Z", // Date part from request, time is zeroed
                "event_time": "11:00:00",
                "organizer_name": "City Events Committee", // Unchanged if not in request
                "venue_name": "Central City Park"
            },
            "housing_details": null,
            "babysitting_details": null
        }
    }
    ```
*   **Error Responses:**
    *   `400 Bad Request`: If the `listing_id` is invalid or the request body has general format issues.
    *   `401 Unauthorized`: If the user is not authenticated.
    *   `403 Forbidden`: If the authenticated user does not own the listing.
    *   `404 Not Found`: If the listing with the specified `listing_id` does not exist.
    *   `422 Unprocessable Entity`: If the request body fails validation (e.g., invalid field values, missing required fields within a details block).
    *   `500 Internal Server Error`: For unexpected server issues.
```
