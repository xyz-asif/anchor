TDR: Unified Search, Discovery Feeds & Final Compliance
1. Objective
To unify the search experience, enable topic-based discovery (Tag Feeds), and fulfill final App Store requirements for account management and engagement feedback.

2. ✅ Pillar 1: Unified Search (The "Find" UX)
Problem: Users currently have to switch tabs to search for Anchors vs. Users vs. Tags. Requirement: Create a single endpoint that aggregates top results for a "Type-ahead" experience. Endpoint: GET /search/combined?q={query}

Logic: Return a JSON object containing:

top_anchors: (Top 3 matches)

top_users: (Top 3 matches)

top_tags: (Top 3 matches)

Goal: Allow the UI to show a mixed result list as the user types.

3. ✅ Pillar 2: Tag Feeds (The "Discovery" UX)
Problem: Tapping a hashtag (e.g., #Photography) currently does nothing. There is no "Topic Landing Page." Requirement: Create an endpoint to fetch all content associated with a specific tag. Endpoint: GET /feed/tags/:tagName

Logic: Return a paginated list of all public anchors containing the specified tag.

Sorting: Default to engagementScore (Popular) to ensure the best content is seen first.

4. ✅ Pillar 3: Engagement & Retention (The "Live" UX)
Problem: The UI cannot show "Unread" badges without fetching the entire notification list (heavy/slow). Requirement: A lightweight counter for the bottom navigation bar. Endpoint: GET /notifications/unread-count

Logic: Return a simple integer of notifications where isRead == false.

5. ✅ Pillar 4: Privacy & Compliance (App Store Mandatory)
Problem: Users cannot manage their blocked list or delete their accounts (Violation of Apple Review Guidelines). Requirements:

GET /users/me/blocks: Return a list of users currently blocked by the requester (for the "Unblock" UI).

DELETE /users/me:

Logic: Perform a "Hard Delete" or "Anonymization" of user data.

Requirement: Must remove/deactivate the user profile and associated anchors as per GDPR/App Store requirements.

🎯 Final Delivery Checklist
[ ] Unified Search: Aggregate data from existing search repositories.

[ ] Tag Feed: Ensure this uses the existing cursor-based pagination logic.

[ ] Unread Count: Optimize query to be highly performant (Index isRead field).

[ ] Hard Delete: Ensure Cloudinary assets are deleted when the account is deleted (to save costs).

[ ] Postman: Final export of all 58+ endpoints.