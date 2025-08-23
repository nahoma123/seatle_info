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

### `DELETE /api/v1/users/me`

*   **Description**: Deletes the currently authenticated user's account and all associated data. This is a destructive and irreversible action.
*   **Auth**: Bearer Token (Firebase ID Token)
*   **Request Body**: None
*   **Headers**:
    *   `Authorization: Bearer <FIREBASE_ID_TOKEN>` (Required)
*   **Response**: `204 No Content`
    *   Indicates the user's account and data have been successfully deleted.
*   **Error Responses**:
    *   `401 Unauthorized`: If the token is missing, invalid, expired, or has been blocklisted.
    *   `500 Internal Server Error`: If an error occurred on the server during the deletion process.

### `GET /api/v1/users`

*   **Description**: Retrieves a paginated list of users. Allows filtering by email, name, and role. This is an admin-only endpoint.
*   **Auth**: Admin (Bearer Token) (Firebase ID Token)
*   **Query Parameters**:
    *   `page` (int, optional, default: 1): The page number for pagination.
    *   `page_size` (int, optional, default: 10): The number of users per page.
    *   `email` (string, optional): Filters users whose email contains the provided string (case-insensitive).
    *   `name` (string, optional): Filters users whose first name or last name contains the provided string (case-insensitive).
    *   `role` (string, optional): Filters users by exact role match (e.g., "user", "admin").
*   **Headers**:
    *   `Authorization: Bearer <FIREBASE_ID_TOKEN>` (Required, must be from an admin user)
*   **Response**: `200 OK`
    ```json
    {
        "message": "Users retrieved successfully.",
        "data": [
            {
                "id": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
                "email": "admin@example.com",
                "first_name": "Admin",
                "last_name": "User",
                "profile_picture_url": "http://example.com/admin.jpg",
                "auth_provider": "firebase",
                "is_email_verified": true,
                "role": "admin",
                "is_first_post_approved": true,
                "created_at": "2023-01-10T09:00:00Z",
                "updated_at": "2023-01-10T09:00:00Z",
                "last_login_at": "2023-10-28T10:00:00Z"
            },
            {
                "id": "b2c3d4e5-f6a7-8901-2345-678901bcdef0",
                "email": "user1@example.com",
                "first_name": "Regular",
                "last_name": "UserOne",
                "profile_picture_url": null,
                "auth_provider": "firebase",
                "is_email_verified": true,
                "role": "user",
                "is_first_post_approved": false,
                "created_at": "2023-02-15T11:00:00Z",
                "updated_at": "2023-02-15T11:00:00Z",
                "last_login_at": "2023-10-27T12:00:00Z"
            }
        ],
        "pagination": {
            "current_page": 1,
            "page_size": 10,
            "total_records": 2,
            "total_pages": 1
        }
    }
    ```
*   **Error Responses**:
    *   `400 Bad Request`: If query parameters are of an invalid type (e.g., non-integer for `page`).
    *   `401 Unauthorized`: If the token is missing, invalid, or not provided.
    *   `403 Forbidden`: If the authenticated user is not an admin.
    *   `500 Internal Server Error`: For unexpected server issues.

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
                "images": [
                    {
                        "id": "img_uuid_example_1",
                        "image_url": "/static/images/listings/armchair_front.jpg",
                        "sort_order": 0
                    }
                ],
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
*   **Content-Type**: `multipart/form-data`
*   **Form Data Parameters**:
    *   `title` (string, required): Title of the listing.
    *   `description` (string, required): Description of the listing.
    *   `category_id` (UUID, required): ID of the category.
    *   `sub_category_id` (UUID, optional): ID of the sub-category.
    *   `contact_name` (string, optional): Contact name.
    *   `contact_email` (string, optional): Contact email.
    *   `contact_phone` (string, optional): Contact phone.
    *   `address_line1` (string, optional): Address line 1.
    *   `address_line2` (string, optional): Address line 2.
    *   `city` (string, optional): City.
    *   `state` (string, optional): State.
    *   `zip_code` (string, optional): Zip code.
    *   `latitude` (float, optional): Latitude.
    *   `longitude` (float, optional): Longitude.
    *   `babysitting_details_json` (string, optional): JSON string for CreateListingBabysittingDetailsRequest. E.g., `{"languages_spoken": ["English", "Spanish"]}`.
    *   `housing_details_json` (string, optional): JSON string for CreateListingHousingDetailsRequest. E.g., `{"property_type": "for_rent", "rent_details": "$1500/month"}`.
    *   `event_details_json` (string, optional): JSON string for CreateListingEventDetailsRequest. E.g., `{"event_date": "2024-12-31", "event_time": "10:00:00"}`.
    *   `images` (file, optional): One or more image files. Use `images` as the field name for each file (e.g., `images` or `images[]` depending on client).
*   **Response**: `201 Created`
    ```json
    {
        "id": "k1j2h3g4-f5e6-d789-c012-b3456789axyz",
        "user_id": "current_authenticated_user_id",
        // ... other listing fields like title, description, category, etc. ...
        "images": [
            {
                "id": "img_uuid_1",
                "image_url": "/static/images/listings/unique_name_1.jpg",
                "sort_order": 0
            },
            {
                "id": "img_uuid_2",
                "image_url": "/static/images/listings/unique_name_2.png",
                "sort_order": 1
            }
        ],
        "status": "active", // Default status
        "created_at": "2023-10-27T15:00:00Z",
        "updated_at": "2023-10-27T15:00:00Z",
        "expires_at": "2023-11-06T15:00:00Z" // Calculated by backend
    }
    ```
*   **Note on Nested Details**: For fields like `babysitting_details`, `housing_details`, and `event_details`, since the main request is `multipart/form-data`, these complex objects should be sent as JSON strings under respective form fields (e.g., `babysitting_details_json`). The backend will parse these JSON strings.
*   **Error Responses**: `400`, `401`, `422`, `500`

### `GET /api/v1/listings/{id}`
*   **Description**: Retrieves a specific listing by its ID.
*   **Auth**: Public (though contact details might be hidden for non-authenticated users or non-owners)
*   **Path Parameters**:
    *   `id` (UUID, required): The ID of the listing to retrieve.
*   **Response**: `200 OK`
    ```json
    {
        "id": "l1m2n3o4-p5q6-r789-s012-t3456789uvwx",
        "title": "Vintage Armchair",
        "description": "Comfortable vintage armchair, good condition.",
        "category_id": "b1c2d3e4-f5a6-b789-0123-456789abcdef",
        "user_id": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
        // ... other fields like price, status, location ...
        "images": [
            {
                "id": "img_uuid_chair_1",
                "image_url": "/static/images/listings/vintage_armchair_1.jpg",
                "sort_order": 0
            },
            {
                "id": "img_uuid_chair_2",
                "image_url": "/static/images/listings/vintage_armchair_2.jpg",
                "sort_order": 1
            }
        ],
        "user": {
            "id": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
            "first_name": "John",
            "last_name": "Doe",
            "profile_picture_url": null
        },
        "category": {
            "id": "b1c2d3e4-f5a6-b789-0123-456789abcdef",
            "name": "Furniture",
            "slug": "furniture"
        },
        // ... babysitting_details, housing_details, or event_details if applicable ...
        "created_at": "2023-10-20T09:00:00Z",
        "updated_at": "2023-10-21T10:00:00Z",
        "expires_at": "2023-11-20T09:00:00Z"
    }
    ```
*   **Error Responses**: `400`, `404`, `500`


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
                "babysitting_details": null,
                "images": [
                    {
                        "id": "img_uuid_fest_1",
                        "image_url": "/static/images/listings/fest_flyer.jpg",
                        "sort_order": 0
                    },
                    {
                        "id": "img_uuid_fest_2",
                        "image_url": "/static/images/listings/fest_crowd.png",
                        "sort_order": 1
                    }
                ]
            }
            // ... other listings ...
        ],
        "pagination": {
            "current_page": 1,
            "page_size": 10,
            "total_records": 1,
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
*   **Content-Type**: `multipart/form-data`
*   **URL Parameters:**
    *   `listing_id` (UUID, required): The ID of the listing to update.
*   **Form Data Parameters**:
    *   Similar to `POST /api/v1/listings`, all fields are optional. Only provided fields will be updated.
    *   `title` (string, optional)
    *   `description` (string, optional)
    *   `contact_name` (string, optional)
    *   `remove_image_ids` (UUID, optional): One or more UUIDs of existing images to remove. Can be sent as repeated form fields (e.g., `remove_image_ids=uuid1&remove_image_ids=uuid2`).
    *   `images` (file, optional): One or more new image files to add.
    *   Category-specific details (e.g. `event_details_json`) can also be updated by sending their JSON string.
    *   The category (`category_id`) of a listing cannot be changed.
    *   `status` and `is_admin_approved` fields are not modifiable via this endpoint.
*   **Successful Response (200 OK):**
    *   Returns the fully updated listing object, including the latest state of its images.
    ```json
    {
        "message": "Listing updated successfully.",
        "data": {
            "id": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
            // ... other updated listing fields ...
            "images": [
                {
                    "id": "img_uuid_1_kept",
                    "image_url": "/static/images/listings/kept_image.jpg",
                    "sort_order": 0
                },
                {
                    "id": "new_img_uuid_3",
                    "image_url": "/static/images/listings/newly_uploaded.png",
                    "sort_order": 1
                }
            ],
            "updated_at": "2024-03-15T14:00:00Z"
        }
    }
    ```
*   **Note on Nested Details**: Similar to POST, send as JSON strings in form fields (e.g. `housing_details_json`).
*   **Error Responses:**
    *   `400 Bad Request`: If the `listing_id` is invalid or the request body has general format issues.
    *   `401 Unauthorized`: If the user is not authenticated.
    *   `403 Forbidden`: If the authenticated user does not own the listing.
    *   `404 Not Found`: If the listing with the specified `listing_id` does not exist.
    *   `422 Unprocessable Entity`: If the request body fails validation (e.g., invalid field values, missing required fields within a details block).
    *   `500 Internal Server Error`: For unexpected server issues.

### `GET /api/v1/listings/recent`
*   **Description**: Fetches a paginated list of the most recently created active and approved listings, excluding items categorized as 'events'.
*   **Auth**: Public
*   **Query Parameters**:
    *   `page` (int, optional, default: 1): The page number for pagination.
    *   `page_size` (int, optional, default: 3): The number of items per page.
*   **Successful Response (200 OK):**
    ```json
    {
        "status": "success",
        "message": "Recent listings retrieved successfully.",
        "data": [
            {
                "id": "uuid-goes-here",
                "title": "Beautiful Handmade Scarf",
                "description": "A warm and cozy handmade scarf, perfect for Seattle winters.",
                "status": "active",
                "created_at": "2023-10-26T10:00:00Z",
                "expires_at": "2023-11-26T10:00:00Z",
                // "contact_name": "Jane Doe",
                // "contact_email": "jane.doe@example.com",
                // "contact_phone": "123-456-7890",
                "address_line1": "123 Main St",
                "city": "Seattle",
                "state": "WA",
                "zip_code": "98101",
                "latitude": 47.6062,
                "longitude": -122.3321,
                "images": [
                    {
                        "id": "img_uuid_scarf_1",
                        "image_url": "/static/images/listings/scarf_closeup.jpg",
                        "sort_order": 0
                    }
                ],
                "category_info": {
                    "id": "category-uuid",
                    "name": "Handmade Goods",
                    "slug": "handmade-goods"
                },
                "user_info": {
                    "id": "user-uuid",
                    "first_name": "John",
                    "last_name": "Doe"
                }
            }
        ],
        "pagination": {
            "current_page": 1,
            "page_size": 3,
            "total_records": 50,
            "total_pages": 17
        }
    }
    ```

---
## Module: Events (Listings subtype)

Manages event-specific listings.

### `GET /api/v1/events/upcoming`

*   **Description**: Fetches a paginated list of upcoming active and approved events, ordered by event date and time.
*   **Auth**: Public
*   **Query Parameters**:
    *   `page` (int, optional, default: 1): The page number for pagination.
    *   `page_size` (int, optional, default: 10): The number of items per page.
*   **Successful Response (200 OK):**
    ```json
    {
        "status": "success",
        "message": "Upcoming events retrieved successfully.",
        "data": [
            {
                "id": "listing-uuid-for-event",
                "title": "Community Music Festival",
                "description": "Join us for a day of live music, food trucks, and fun!",
                "status": "active",
                "created_at": "2023-10-01T14:30:00Z",
                "expires_at": "2023-11-16T00:00:00Z",
                "contact_name": "Event Organizer Co.",
                "contact_email": "info@eventfest.com",
                "contact_phone": "555-123-4567",
                "address_line1": "456 Park Ave",
                "city": "Seattle",
                "state": "WA",
                "zip_code": "98104",
                "latitude": 47.6000,
                "longitude": -122.3300,
                "event_details": {
                    "event_date": "2023-11-15",
                    "event_time": "12:00:00",
                    "organizer_name": "Community Events LLC",
                    "venue_name": "City Park Amphitheater"
                },
                "images": [
                     {
                        "id": "img_uuid_event_1",
                        "image_url": "/static/images/listings/event_poster.jpg",
                        "sort_order": 0
                    }
                ],
                "category_info": {
                    "id": "category-uuid-events",
                    "name": "Events",
                    "slug": "events"
                },
                "user_info": {
                    "id": "user-uuid-creator",
                    "first_name": "Alice",
                    "last_name": "Smith"
                }
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

---
## Module: Notifications

All notification endpoints require Bearer Token authentication.

### `GET /api/v1/notifications`

*   **Description**: Fetches a paginated list of notifications for the authenticated user, ordered by creation date (newest first).
*   **Auth**: Bearer Token (Firebase ID Token)
*   **Query Parameters**:
    *   `page` (int, optional, default: 1): The page number.
    *   `page_size` (int, optional, default: 10): Items per page.
*   **Successful Response (200 OK):**
    ```json
    {
        "status": "success",
        "message": "Notifications retrieved successfully.",
        "data": [
            {
                "id": "notification-uuid-1",
                "user_id": "authenticated-user-uuid",
                "type": "listing_approved_live",
                "message": "Great news! Your listing 'My Awesome Event' has been approved and is now live.",
                "related_listing_id": "listing-uuid-for-event",
                "is_read": false,
                "created_at": "2023-10-26T12:00:00Z"
            },
            {
                "id": "notification-uuid-2",
                "user_id": "authenticated-user-uuid",
                "type": "listing_created_pending_approval",
                "message": "Your listing 'New Item for Sale' has been submitted and is pending review.",
                "related_listing_id": "listing-uuid-for-item",
                "is_read": true,
                "created_at": "2023-10-25T10:00:00Z"
            }
        ],
        "pagination": {
            "current_page": 1,
            "page_size": 10,
            "total_records": 5,
            "total_pages": 1
        }
    }
    ```

### `POST /api/v1/notifications/{notification_id}/mark-read`

*   **Description**: Marks a specific notification as read for the authenticated user. The user must be the owner of the notification.
*   **Auth**: Bearer Token (Firebase ID Token)
*   **Path Parameter**:
    *   `notification_id` (UUID): The ID of the notification to mark as read.
*   **Request Body**: None
*   **Successful Response (200 OK):**
    ```json
    {
        "status": "success",
        "message": "Notification marked as read successfully.",
        "data": null
    }
    ```
*   **Error Responses**:
    *   `401 Unauthorized`: If token is missing or invalid.
    *   `403 Forbidden`: If the notification does not belong to the user.
    *   `404 Not Found`: If the notification ID does not exist.

### `POST /api/v1/notifications/mark-all-read`

*   **Description**: Marks all unread notifications for the authenticated user as read.
*   **Auth**: Bearer Token (Firebase ID Token)
*   **Request Body**: None
*   **Successful Response (200 OK):**
    ```json
    {
        "status": "success",
        "message": "All notifications marked as read successfully.",
        "data": null
    }
    ```
*   **Error Responses**:
    *   `401 Unauthorized`: If token is missing or invalid.

---
