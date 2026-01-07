package notifications

func ValidateNotificationListQuery(query *NotificationListQuery) error {
	if query.Page < 1 {
		query.Page = 1
	}

	if query.Limit < 1 {
		query.Limit = 20
	}
	if query.Limit > 50 {
		query.Limit = 50
	}

	return nil
}
