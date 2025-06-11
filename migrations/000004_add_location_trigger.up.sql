CREATE OR REPLACE FUNCTION update_location_column()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.longitude IS NOT NULL AND NEW.latitude IS NOT NULL THEN
        NEW.location = ST_SetSRID(ST_MakePoint(NEW.longitude, NEW.latitude), 4326);
    ELSE
        NEW.location = NULL;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER before_insert_or_update_listings_set_location
BEFORE INSERT OR UPDATE ON listings
FOR EACH ROW
EXECUTE FUNCTION update_location_column();
