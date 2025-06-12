# Manual Testing Guide: `sync-listings` Utility

This document outlines the steps to manually test the `sync-listings` command-line utility, which synchronizes listing data from the PostgreSQL database to Elasticsearch.

## 1. Environment Setup

*   **Start Services:**
    *   Ensure PostgreSQL and Elasticsearch Docker containers are running. From the project root, execute:
        ```bash
        docker-compose -f docker-compose.dev.yml up -d postgres_db elasticsearch
        ```
    *   Wait a few moments for the services to initialize fully.
*   **Verify/Prepare Elasticsearch Index:**
    *   The `sync-listings` utility (and the main application) is designed to create the `listings` index with the correct mapping if it doesn't exist.
    *   For a clean test run, you can optionally delete any pre-existing `listings` index. **Caution: This will delete any data currently in that index.**
        ```bash
        curl -X DELETE "http://localhost:9200/listings"
        ```
        If the index didn't exist, this command will return a "not found" error, which is acceptable. The utility will create it in the next step.

## 2. Prepare Test Data in PostgreSQL

For this test, you'll need to insert sample data directly into the PostgreSQL database.

*   **Connect to PostgreSQL:**
    *   You can use `psql`, a GUI tool (like DBeaver or pgAdmin connected to `localhost:5432`, using credentials from your `.env` file), or any other method to connect to the `seattle_info_db` database.
    *   The default credentials (if using `.env.example` directly) are typically `DB_USER=your_db_user`, `DB_PASSWORD=your_db_password`.

*   **Insert Sample Data:**
    *   **Important:** Before inserting listings, ensure you have existing records in the `users` and `categories` (and `sub_categories` if used) tables. The `listings` table has foreign key constraints to these.
    *   You may need to create sample users and categories first if your database is empty. For example:
        ```sql
        -- Example: Create a user (replace with actual UUIDs or generate them)
        INSERT INTO users (id, username, email, password_hash, role, is_active, is_email_verified, created_at, updated_at)
        VALUES ('some-uuid-for-user-1', 'testsyncuser', 'sync@example.com', '$2a$10$...', 'user', true, true, NOW(), NOW());

        -- Example: Create a category
        INSERT INTO categories (id, name, slug, description, created_at, updated_at)
        VALUES ('some-uuid-for-category-1', 'Services', 'services', 'Various local services', NOW(), NOW());

        -- Example: Create a sub-category
        INSERT INTO sub_categories (id, category_id, name, slug, description, created_at, updated_at)
        VALUES ('some-uuid-for-subcategory-1', 'some-uuid-for-category-1', 'Plumbing', 'plumbing', 'Plumbing services', NOW(), NOW());
        ```
    *   Insert 5-10 diverse listings into the `listings` table. Here are examples (ensure you use valid UUIDs for `id`, `user_id`, `category_id`, and `sub_category_id` that exist in their respective tables):

        ```sql
        -- Listing 1: Active, with geo-location, no sub-category
        INSERT INTO listings (id, user_id, category_id, title, description, status, expires_at, is_admin_approved, contact_name, city, state, zip_code, address_line1, latitude, longitude, created_at, updated_at) VALUES
        ('listing-uuid-1', 'user-uuid-1', 'category-uuid-services', 'Awesome Plumber in Seattle', 'Reliable plumbing services for downtown.', 'active', NOW() + INTERVAL '30 days', true, 'John Plumb', 'Seattle', 'WA', '98101', '123 Main St', 47.6062, -122.3321, NOW(), NOW());

        -- Listing 2: Pending, with sub-category, different user
        INSERT INTO listings (id, user_id, category_id, sub_category_id, title, description, status, expires_at, is_admin_approved, contact_name, city, state, zip_code, address_line1, created_at, updated_at) VALUES
        ('listing-uuid-2', 'user-uuid-2', 'category-uuid-services', 'subcategory-uuid-plumbing', 'Emergency Pipe Repair', '24/7 emergency pipe repairs.', 'pending_approval', NOW() + INTERVAL '60 days', true, 'Pipe Masters', 'Seattle', 'WA', '98102', '456 Pine St', NOW(), NOW());

        -- Listing 3: Event, with event details
        INSERT INTO listings (id, user_id, category_id, title, description, status, expires_at, is_admin_approved, city, state, created_at, updated_at) VALUES
        ('listing-uuid-3', 'user-uuid-1', 'category-uuid-events', 'Community Fair', 'Annual community fair with games and food.', 'active', NOW() + INTERVAL '10 days', true, 'Seattle', 'WA', NOW(), NOW());
        INSERT INTO listing_details_events (listing_id, event_date, event_time, organizer_name, venue_name) VALUES
        ('listing-uuid-3', NOW() + INTERVAL '7 days', '10:00:00', 'City Events Co', 'Community Park');

        -- Listing 4: Housing, with housing details
        INSERT INTO listings (id, user_id, category_id, title, description, status, expires_at, is_admin_approved, city, state, created_at, updated_at) VALUES
        ('listing-uuid-4', 'user-uuid-2', 'category-uuid-housing', 'Apartment for Rent', '2 bed apartment available.', 'active', NOW() + INTERVAL '45 days', true, 'Seattle', 'WA', NOW(), NOW());
        INSERT INTO listing_details_housing (listing_id, property_type, rent_details) VALUES
        ('listing-uuid-4', 'for_rent', '1500 USD/month, utilities included');

        -- Listing 5: Babysitting, with details
        INSERT INTO listings (id, user_id, category_id, title, description, status, expires_at, is_admin_approved, city, state, created_at, updated_at) VALUES
        ('listing-uuid-5', 'user-uuid-1', 'category-uuid-babysitting', 'Evening Babysitter', 'Experienced babysitter available evenings.', 'active', NOW() + INTERVAL '20 days', true, 'Seattle', 'WA', NOW(), NOW());
        INSERT INTO listing_details_babysitting (listing_id, languages_spoken) VALUES
        ('listing-uuid-5', '{"English", "Spanish"}');

        -- Add a few more to reach 5-10, varying data points.
        ```
    *   Replace `'user-uuid-...'`, `'category-uuid-...'`, `'listing-uuid-...'` etc. with actual valid UUIDs.

## 3. Run the Synchronization Utility

*   Navigate to the project's root directory in your terminal.
*   Execute the `sync-listings` command. Using a small batch size helps observe the batching process. Setting refresh to true makes data immediately available for querying in Elasticsearch.
    ```bash
    go run ./cmd/server/main.go sync-listings --batch-size=2 --es-refresh=true
    ```
*   **Monitor Console Output:**
    *   Look for logs indicating successful connection to DB and Elasticsearch.
    *   Confirm logs showing the `listings` index being checked/created.
    *   Observe batch processing messages: "Fetching batch of listings...", "Fetched listings for batch...", "Sending bulk request...", "Batch processed...".
    *   Note any error messages related to individual document indexing or entire batch failures.
    *   Check the final summary: "Listing synchronization process finished. Total listings synced successfully: X, Total listings failed: Y".

## 4. Verify Data in Elasticsearch

Use Kibana Dev Tools or `curl` commands to interact with Elasticsearch and verify the synchronized data.

*   **Count Documents:**
    Check if the total number of documents in the `listings` index matches the number of listings you inserted into PostgreSQL.
    ```bash
    curl -X GET "http://localhost:9200/listings/_count"
    ```
    Expected: `{"count":N,...}` where N is the number of listings.

*   **Retrieve Specific Listings:**
    Fetch a few listings by their UUIDs (use the UUIDs you inserted).
    ```bash
    curl -X GET "http://localhost:9200/listings/_doc/<your_listing_uuid_1>"
    curl -X GET "http://localhost:9200/listings/_doc/<your_listing_uuid_2>"
    ```
*   **Inspect `_source`:**
    For the retrieved documents, carefully examine the `_source` field. Verify:
    *   All expected fields from your `esutil.ListingToElasticsearchDoc` function are present (e.g., `title`, `description`, `status`, `category_id`, `user_id`, `user_username`, `category_name`, `city`, `state`, `zip_code`, `address_line1`, `is_admin_approved`).
    *   `location` field exists and has the correct `{"lat": ..., "lon": ...}` structure for listings with coordinates. It should be `null` or absent for those without.
    *   Category-specific details (`languages_spoken`, `property_type`, `event_date`, etc.) are correctly included.
    *   Timestamps like `expires_at`, `created_at`, `updated_at` are in a valid date format (Elasticsearch default is `yyyy-MM-dd'T'HH:mm:ss.SSS'Z'` or epoch milliseconds).

*   **Basic Search:**
    Perform a keyword search using a term present in one or more of your test listings' titles or descriptions.
    ```bash
    curl -X GET "http://localhost:9200/listings/_search?q=Awesome"
    # or for a specific field search:
    # curl -X GET "http://localhost:9200/listings/_search?q=title:Awesome"
    ```
    Confirm that the relevant listing(s) are returned in the `hits.hits` array.

*   **Geo-distance Search (Optional, Advanced):**
    If you have listings with locations and want to test geo search directly in Elasticsearch:
    ```json
    // Example: Find listings within 2km of a point
    // POST /listings/_search
    // {
    //   "query": {
    //     "bool": {
    //       "filter": {
    //         "geo_distance": {
    //           "distance": "2km",
    //           "location": {
    //             "lat": 47.6062,
    //             "lon": -122.3321
    //           }
    //         }
    //       }
    //     }
    //   }
    // }
    ```
    Verify that the `location` field in your documents has the `geo_point` mapping (which `CreateListingsIndexIfNotExists` should have set up).

## 5. Test Idempotency (Optional but Recommended)

*   **Re-run the Sync Command:**
    Execute the same sync command again:
    ```bash
    go run ./cmd/server/main.go sync-listings --batch-size=2 --es-refresh=true
    ```
*   **Verify:**
    *   The command should complete without errors.
    *   The console output should indicate that listings were processed (Elasticsearch `index` operations will update existing documents if the content is the same or different).
    *   The total document count in Elasticsearch (`GET /listings/_count`) should remain the same (no duplicates).
    *   Retrieve a document that was present before the re-run and ensure its content is still correct and its `_version` number might have increased by 1 (if content was identical, ES might optimize and not increment version, but if re-indexed, it will).

By following these steps, you can thoroughly test the `sync-listings` utility and ensure data is correctly transferred and represented in Elasticsearch.
