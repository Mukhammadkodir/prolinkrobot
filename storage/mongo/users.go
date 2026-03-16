package mongo

import (
	"context"
	"fmt"
	"get-link-tg-bot/config"
	"get-link-tg-bot/models"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const mongoTimeout = 10 * time.Second

type Users struct {
	cfg             *config.Config
	client          *mongo.Client
	collection      *mongo.Collection
	assetCollection *mongo.Collection
	location        *time.Location
}

func NewUsers(cfg *config.Config) (*Users, error) {
	location, err := time.LoadLocation(cfg.Limits.Timezone)
	if err != nil {
		return nil, fmt.Errorf("load timezone: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), mongoTimeout)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.Mongo.URI))
	if err != nil {
		return nil, fmt.Errorf("connect mongo: %w", err)
	}

	pingCtx, pingCancel := context.WithTimeout(context.Background(), mongoTimeout)
	defer pingCancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		return nil, fmt.Errorf("ping mongo: %w", err)
	}

	store := &Users{
		cfg:             cfg,
		client:          client,
		collection:      client.Database(cfg.Mongo.Database).Collection(cfg.Mongo.TelegramUsersCollection),
		assetCollection: client.Database(cfg.Mongo.Database).Collection(cfg.Cache.AssetCollection),
		location:        location,
	}

	if err := store.ensureIndexes(); err != nil {
		return nil, err
	}
	return store, nil
}

func (u *Users) ensureIndexes() error {
	ctx, cancel := u.operationContext()
	defer cancel()

	_, err := u.collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "tg_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	})
	if err != nil {
		return fmt.Errorf("ensure indexes: %w", err)
	}

	_, err = u.assetCollection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "key", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	})
	if err != nil {
		return fmt.Errorf("ensure asset cache indexes: %w", err)
	}
	return nil
}

func (u *Users) UserExists(tgID int64) (bool, error) {
	ctx, cancel := u.operationContext()
	defer cancel()

	count, err := u.collection.CountDocuments(ctx, bson.M{"tg_id": tgID}, options.Count().SetLimit(1))
	if err != nil {
		return false, fmt.Errorf("count user: %w", err)
	}
	return count > 0, nil
}

func (u *Users) GetUserByTelegramID(tgID int64) (*models.TelegramUser, error) {
	ctx, cancel := u.operationContext()
	defer cancel()

	var user models.TelegramUser
	err := u.collection.FindOne(ctx, bson.M{"tg_id": tgID}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}
	u.normalizeUserDefaults(&user)
	return &user, nil
}

func (u *Users) SaveTelegramUser(req *models.SaveTelegramUserRequest) error {
	now := time.Now().In(u.location)
	ctx, cancel := u.operationContext()
	defer cancel()

	update := bson.M{
		"$setOnInsert": bson.M{
			"tg_id":                req.TelegramID,
			"name":                 req.Name,
			"username":             req.Username,
			"lang":                 req.Language,
			"daily_limit":          defaultIfZero(req.DailyLimit, u.cfg.Limits.DefaultDailyLimit),
			"limit_date":           req.LimitDate,
			"downloads_today":      0,
			"downloads_reset_date": u.todayString(now),
			"total_downloads":      0,
			"create_date":          now,
			"update_date":          now,
		},
		"$set": bson.M{
			"name":        req.Name,
			"username":    req.Username,
			"update_date": now,
		},
	}

	_, err := u.collection.UpdateOne(ctx, bson.M{"tg_id": req.TelegramID}, update, options.Update().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("save telegram user: %w", err)
	}
	return nil
}

func (u *Users) UpdateUserLanguage(tgID int64, language string) error {
	ctx, cancel := u.operationContext()
	defer cancel()

	_, err := u.collection.UpdateOne(
		ctx,
		bson.M{"tg_id": tgID},
		bson.M{"$set": bson.M{"lang": language, "update_date": time.Now().In(u.location)}},
	)
	if err != nil {
		return fmt.Errorf("update user language: %w", err)
	}
	return nil
}

func (u *Users) GetCachedAssetByKey(key string) (*models.AssetCacheEntry, error) {
	ctx, cancel := u.operationContext()
	defer cancel()

	var entry models.AssetCacheEntry
	err := u.assetCollection.FindOne(ctx, bson.M{"key": key}).Decode(&entry)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get cached asset: %w", err)
	}
	return &entry, nil
}

func (u *Users) SaveCachedAsset(key, sourceURL string, channelChatID int64, channelMessageID int, assetType string) error {
	ctx, cancel := u.operationContext()
	defer cancel()

	now := time.Now().In(u.location)
	_, err := u.assetCollection.UpdateOne(
		ctx,
		bson.M{"key": key},
		bson.M{
			"$set": bson.M{
				"source_url":         sourceURL,
				"channel_chat_id":    channelChatID,
				"channel_message_id": channelMessageID,
				"asset_type":         assetType,
				"created_at":         now,
			},
			"$setOnInsert": bson.M{"key": key},
		},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("save cached asset: %w", err)
	}
	return nil
}

func (u *Users) DeleteCachedAssetByKey(key string) error {
	ctx, cancel := u.operationContext()
	defer cancel()

	_, err := u.assetCollection.DeleteOne(ctx, bson.M{"key": key})
	if err != nil {
		return fmt.Errorf("delete cached asset: %w", err)
	}
	return nil
}

func (u *Users) TryIncrementDownload(tgID int64) (*models.DownloadAttempt, error) {
	user, err := u.GetUserByTelegramID(tgID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return &models.DownloadAttempt{Allowed: false, ErrorMessage: "User not found"}, nil
	}

	user, err = u.ensureUserDailyCounterCurrent(tgID, user)
	if err != nil {
		return nil, err
	}

	now := time.Now().In(u.location)
	downloadsToday := user.DownloadsToday
	dailyLimit := defaultIfZero(user.DailyLimit, u.cfg.Limits.DefaultDailyLimit)

	isPremium := false
	if user.LimitDate != nil {
		limitDate := user.LimitDate.In(u.location)
		if now.Before(limitDate) {
			isPremium = true
		}
	}

	if !isPremium {
		trialEnd := user.CreateDate.In(u.location).AddDate(0, 0, u.cfg.Limits.TrialPeriodDays)
		if now.After(trialEnd) {
			return &models.DownloadAttempt{
				Allowed:        false,
				DownloadsToday: downloadsToday,
				DailyLimit:     dailyLimit,
				ErrorMessage:   "Trial period expired",
			}, nil
		}
	}

	if downloadsToday >= dailyLimit {
		return &models.DownloadAttempt{
			Allowed:        false,
			DownloadsToday: downloadsToday,
			DailyLimit:     dailyLimit,
			ErrorMessage:   "Daily limit reached",
		}, nil
	}

	ctx, cancel := u.operationContext()
	defer cancel()

	result, err := u.collection.UpdateOne(
		ctx,
		bson.M{
			"tg_id":           tgID,
			"downloads_today": bson.M{"$lt": dailyLimit},
		},
		bson.M{
			"$inc": bson.M{"downloads_today": 1, "total_downloads": 1},
			"$set": bson.M{"update_date": now},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("increment download: %w", err)
	}

	if result.ModifiedCount > 0 {
		return &models.DownloadAttempt{
			Allowed:        true,
			DownloadsToday: downloadsToday + 1,
			DailyLimit:     dailyLimit,
		}, nil
	}

	user, err = u.GetUserByTelegramID(tgID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return &models.DownloadAttempt{Allowed: false, ErrorMessage: "User not found"}, nil
	}

	return &models.DownloadAttempt{
		Allowed:        false,
		DownloadsToday: user.DownloadsToday,
		DailyLimit:     dailyLimit,
		ErrorMessage:   "Daily limit reached",
	}, nil
}

func (u *Users) DecrementDownload(tgID int64) error {
	ctx, cancel := u.operationContext()
	defer cancel()

	_, err := u.collection.UpdateOne(
		ctx,
		bson.M{"tg_id": tgID, "downloads_today": bson.M{"$gt": 0}},
		bson.M{
			"$inc": bson.M{"downloads_today": -1, "total_downloads": -1},
			"$set": bson.M{"update_date": time.Now().In(u.location)},
		},
	)
	if err != nil {
		return fmt.Errorf("decrement download: %w", err)
	}
	return nil
}

func (u *Users) GetUserStats(tgID int64) (*models.UserStats, error) {
	user, err := u.GetUserByTelegramID(tgID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
	}

	user, err = u.ensureUserDailyCounterCurrent(tgID, user)
	if err != nil {
		return nil, err
	}

	createDate := user.CreateDate.In(u.location)
	trialEnd := createDate.AddDate(0, 0, u.cfg.Limits.TrialPeriodDays)

	stats := &models.UserStats{
		Name:           user.Name,
		Username:       user.Username,
		DownloadsToday: user.DownloadsToday,
		TotalDownloads: user.TotalDownloads,
		DailyLimit:     defaultIfZero(user.DailyLimit, u.cfg.Limits.DefaultDailyLimit),
		LimitDate:      user.LimitDate,
		CreateDate:     &createDate,
		TrialEnd:       &trialEnd,
	}
	return stats, nil
}

func (u *Users) UpdateUserLimit(tgID int64, dailyLimit int, limitDate *time.Time) (bool, error) {
	ctx, cancel := u.operationContext()
	defer cancel()

	updateData := bson.M{
		"daily_limit": defaultIfZero(dailyLimit, u.cfg.Limits.DefaultDailyLimit),
		"update_date": time.Now().In(u.location),
	}
	if limitDate != nil {
		updateData["limit_date"] = limitDate.In(u.location)
	}

	result, err := u.collection.UpdateOne(ctx, bson.M{"tg_id": tgID}, bson.M{"$set": updateData})
	if err != nil {
		return false, fmt.Errorf("update user limit: %w", err)
	}
	return result.MatchedCount > 0, nil
}

func (u *Users) ResetDailyLimits() (int64, error) {
	now := time.Now().In(u.location)
	ctx, cancel := u.operationContext()
	defer cancel()

	result, err := u.collection.UpdateMany(
		ctx,
		bson.M{},
		bson.M{"$set": bson.M{"downloads_today": 0, "downloads_reset_date": u.todayString(now), "update_date": now}},
	)
	if err != nil {
		return 0, fmt.Errorf("reset daily limits: %w", err)
	}
	return result.ModifiedCount, nil
}

func (u *Users) ensureUserDailyCounterCurrent(tgID int64, user *models.TelegramUser) (*models.TelegramUser, error) {
	now := time.Now().In(u.location)
	today := u.todayString(now)
	if normalizeResetDate(user.DownloadsResetDate) == today {
		return user, nil
	}

	ctx, cancel := u.operationContext()
	defer cancel()

	_, err := u.collection.UpdateOne(
		ctx,
		bson.M{"tg_id": tgID, "downloads_reset_date": bson.M{"$ne": today}},
		bson.M{"$set": bson.M{"downloads_today": 0, "downloads_reset_date": today, "update_date": now}},
	)
	if err != nil {
		return nil, fmt.Errorf("lazy reset daily counter: %w", err)
	}

	user.DownloadsToday = 0
	user.DownloadsResetDate = today
	user.UpdateDate = now
	return user, nil
}

func (u *Users) normalizeUserDefaults(user *models.TelegramUser) {
	if user.DailyLimit == 0 {
		user.DailyLimit = u.cfg.Limits.DefaultDailyLimit
	}
	if user.DownloadsResetDate == "" {
		user.DownloadsResetDate = u.todayString(time.Now().In(u.location))
	}
}

func (u *Users) todayString(now time.Time) string {
	return now.In(u.location).Format("2006-01-02")
}

func (u *Users) operationContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), mongoTimeout)
}

func normalizeResetDate(value string) string {
	return value
}

func defaultIfZero(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}
