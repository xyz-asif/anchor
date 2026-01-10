Technical Design Requirement: Media, Onboarding & Safety Enhancements
1. Project Context
Following the successful completion of the Core Anchor Handler (Step 4), we have identified critical "Cold Start" and "Compliance" gaps. These updates are required to support the mobile UI's "Quick Add" flow, "Interest Picker," and App Store safety requirements.

2. ✅ Media & Cloudinary Enhancements
Issue A: Standalone Uploads

Requirement: Decouple file uploads from Anchor item creation. This is needed for user avatars and draft states. Action: Create POST /media/upload handler using the existing cloudinary.Service.

Go
// Expected Response
{
    "url": "https://res.cloudinary.com/...",
    "public_id": "anchor/images/abc123",
    "resource_type": "image|video|raw"
}
Issue B: Audio Metadata

Requirement: Extract audio duration for the UI mini-player. File: internal/pkg/cloudinary/cloudinary.go Action: Map the duration field from the Cloudinary response.

Go
// Update in UploadAudio
result, err := s.cld.Upload.Upload(ctx, file, uploadParams)
// ...
return &UploadResult{
    URL:      result.SecureURL,
    Duration: result.Duration, // Ensure this is mapped from the SDK response map
    FileSize: int64(result.Bytes),
}, nil
Issue C: Link Preview (Scraper)

Requirement: Wire the existing scraper to a public endpoint for "Quick Add" previews. Action: Create GET /media/preview?url=... Logic: Call FetchURLMetadata(url) from scraper.go and return the result as JSON.

3. ✅ Onboarding & Interests
Issue D: Persisting User Interests

Requirement: Users can select interests, but we cannot save them. Action: Implement POST /users/me/interests. Model:

Go
type SaveInterestsRequest struct {
    Tags []string `json:"tags" binding:"required,min=1,max=10"`
}
Logic: Update the User document with these tags to seed the engagementScore algorithm.

4. ✅ Safety & Compliance (App Store Mandatory)
Issue E: Report & Block System

Requirement: Apple/Google require a mechanism to report content and block users. Action: Create the following:

POST /reports:

Fields: targetId (Anchor or Item ID), targetType, reason.

POST /users/:id/block:

Logic: Add targetUserId to the current user's blockedUsers array.

Filter: Update repository.go to exclude blocked users from all feed queries.

5. ✅ Summary of New Endpoints Needed
Endpoint	Method	Purpose	Priority
/media/upload	POST	Standalone file/avatar upload	High
/media/preview	GET	Rich link preview for Quick Add	Medium
/users/me/interests	POST	Save onboarding selections	High
/reports	POST	Report abusive content	CRITICAL
/users/:id/block	POST	Block a user	CRITICAL
🎯 Implementation Checklist for Antigravity
[ ] Uncomment/Wire Scraper: Connect scraper.go to the new /media/preview route.

[ ] Update Cloudinary Service: Map Duration in the UploadResult struct.

[ ] Create Safety Models: Add Report and Block schemas to the database.

[ ] Async Notifications: Ensure notificationService continues to run in Goroutines for AddItem to maintain UI speed.

[ ] Refresh Postman: Export a new .json collection including these 5 new endpoints.