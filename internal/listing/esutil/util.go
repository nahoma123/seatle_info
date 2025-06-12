package esutil

import (
	"encoding/json"
	"errors"
	"fmt"
	"time" // Required for date formatting if any

	"seattle_info_backend/internal/listing" // Corrected import path for Listing model

	"github.com/google/uuid"
)

// ListingToElasticsearchDoc converts a listing.Listing object to its Elasticsearch document representation.
// It expects all necessary associations (User, Category, SubCategory, Details) to be preloaded on the listing object.
func ListingToElasticsearchDoc(l *listing.Listing) (string, error) { // Changed param type to listing.Listing
	if l == nil {
		return "", errors.New("listing cannot be nil")
	}

	doc := map[string]interface{}{
		"title":             l.Title,
		"description":       l.Description,
		"category_id":       l.CategoryID.String(),
		"user_id":           l.UserID.String(),
		"status":            string(l.Status),
		"expires_at":        l.ExpiresAt,
		"created_at":        l.CreatedAt,
		"updated_at":        l.UpdatedAt,
		"is_admin_approved": l.IsAdminApproved,
		"contact_name":      l.ContactName,
		"city":              l.City,
		"state":             l.State,
		"zip_code":          l.ZipCode,
		"address_line1":     l.AddressLine1,
	}

	if l.User != nil {
		doc["user_username"] = l.User.Username
	}

	if l.Category.Name != "" {
		doc["category_name"] = l.Category.Name
		doc["category_slug"] = l.Category.Slug
	}
	if l.SubCategory != nil {
		doc["sub_category_id"] = l.SubCategoryID.String()
		doc["sub_category_name"] = l.SubCategory.Name
		doc["sub_category_slug"] = l.SubCategory.Slug
	} else {
		doc["sub_category_id"] = nil
	}

	if l.Latitude != nil && l.Longitude != nil {
		doc["location"] = map[string]float64{
			"lat": *l.Latitude,
			"lon": *l.Longitude,
		}
	} else {
		doc["location"] = nil
	}

	if l.BabysittingDetails != nil {
		doc["languages_spoken"] = l.BabysittingDetails.LanguagesSpoken
	}
	if l.HousingDetails != nil {
		doc["property_type"] = l.HousingDetails.PropertyType
		if l.HousingDetails.RentDetails != nil {
			doc["rent_details"] = *l.HousingDetails.RentDetails
		}
		if l.HousingDetails.SalePrice != nil {
			doc["sale_price"] = *l.HousingDetails.SalePrice
		}
	}
	if l.EventDetails != nil {
		if !l.EventDetails.EventDate.IsZero() {
			doc["event_date"] = l.EventDetails.EventDate.Format("2006-01-02")
		}
		doc["event_time"] = l.EventDetails.EventTime
		doc["organizer_name"] = l.EventDetails.OrganizerName
		doc["venue_name"] = l.EventDetails.VenueName
	}

	docBytes, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("error marshalling listing to JSON for ES: %w", err)
	}
	return string(docBytes), nil
}
