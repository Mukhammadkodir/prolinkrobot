package messages

import (
	"html"
	"strconv"
	"strings"
)

var translations = map[string]map[string]string{
	"en": {
		"choose_language":            "🌍 Please choose your language:\n\nПожалуйста, выберите язык\nTilni tanlang",
		"language_selected":          "✅ Language set to English",
		"welcome":                    "👋 Welcome to Freepik Download Bot!\n🆔 Your Telegram ID: {}\n\nDear customer, you have the opportunity to download 1 file per day for the first week. If you need more limit, contact the Admin.\nAdmin: @ProlinkAdmin\n\nSend a Freepik link to the bot and it will give you a direct download link.\n\nSupported types:\n✅ Icons, Vectors, Photos\n✅ PSD Files, Templates\n✅ Mockups, Illustrations\n\nFor more information, press /help.",
		"help":                       "ℹ️ User Guide:\n\n1️⃣ Send any Freepik premium link to the bot\n2️⃣ Wait for the bot to process it (30-60 seconds)\n3️⃣ Receive the direct download link\n4️⃣ Click the link to download the file\n\nCommands:\n/start - Start the bot\n/language - Change language\n/help - Show help message\n/check_limit - View your limits",
		"help_admin":                 "ℹ️ Admin Guide:\n\n1️⃣ Send any Freepik premium link to the bot\n2️⃣ Wait for the bot to process it (30-60 seconds)\n3️⃣ Receive the direct download link\n4️⃣ Click the link to download the file\n\n👥 User Commands:\n/start - Start the bot\n/language - Change language\n/help - Show help message\n/check_limit - View your limits\n\n🔑 Admin Commands:\n/set_limit - Set daily limit for user\n/cache_mode_on - Enable cache import mode\n/cache_mode_off - Disable cache import mode\n/cache_mode_status - Show cache queue\n/cache_mode_clear - Clear cache queue\n/cancel - Cancel active admin flow",
		"processing":                 "🔍 *Processing your Freepik link...*\n\n⏳ This may take 30-60 seconds. Please wait...",
		"error":                      "❌ Sorry, could not extract the download link.\n\nPlease check if the link is correct. If the problem is not with the link, contact admin.",
		"temporarily_unavailable":    "⏳ The service is temporarily unavailable.\n\nPlease try again a little later.",
		"unsupported_type":           "⚠️ *This content type is not yet supported*\n\nThe bot cannot download {} at the moment.\n\n💬 If you need this file, please contact admin.",
		"no_url":                     "❌ No valid Freepik URL found in your message.\n\nPlease send a valid Freepik URL.\nExample: https://www.freepik.com/premium-psd/your-design_1234567.htm\n\nType /help for more information.",
		"limit_reached":              "❌ *Daily Limit Reached*\n\nYou have reached your daily download limit.\n\n📊 Today: {}/{}\n\nPlease try again tomorrow or contact admin for more downloads.",
		"trial_expired":              "⚠️ *Trial Period Expired*\n\nYour 7-day trial has ended.\n\n📊 Today: {}/{}\n\nPlease contact admin for premium access.",
		"check_limit_title":          "📊 Your Limit:",
		"check_limit_name":           "👤 Name",
		"check_limit_username":       "🆔 Username",
		"check_limit_stats":          "📅 Download Statistics",
		"check_limit_today":          "• Today",
		"check_limit_total":          "• Total",
		"check_limit_status":         "Status",
		"check_limit_premium":        "⭐ Premium",
		"check_limit_premium_until":  "📆 Premium until",
		"check_limit_daily_limit":    "♻️ Daily Limit",
		"check_limit_trial":          "🆓 Trial Period",
		"check_limit_trial_ends":     "⏳ Trial ends",
		"check_limit_days_left":      "days left",
		"check_limit_expired":        "⚠️ Trial Period Expired",
		"check_limit_contact_admin":  "💬 Contact admin @ProlinkAdmin for premium",
		"check_limit_registered":     "📅 Registered",
		"check_limit_reset_info":     "🔄 Limits reset daily at 00:00 (Tashkent time)",
		"check_limit_not_registered": "❌ You are not registered yet. Use /start to register.",
		"check_limit_error":          "❌ Error retrieving your limit information. Please try again later.",
		"set_limit_start":            "👑 Admin: Set User Limit\n\n📅 Please send the limit end date in format: YYYY.MM.DD\n\nExample: 2026.12.31\n\nOr send 0 for no expiration date (trial users only).",
		"set_limit_bad_date":         "❌ Invalid date format. Please use: YYYY.MM.DD\n\nExample: 2026.12.31\nOr send 0 for no expiration.",
		"set_limit_past_date":        "❌ Date must be in the future. Please try again.\n\nFormat: YYYY.MM.DD (e.g., 2026.12.31)",
		"set_limit_ask_daily":        "✅ Date set!\n\n🔢 Now send the daily download limit (number):\n\nExample: 10 for 10 downloads per day\nOr 999 for unlimited",
		"set_limit_bad_daily":        "❌ Invalid number. Please send a number like 10 or 999.",
		"set_limit_ask_user":         "✅ Daily limit set!\n\n👤 Now send the user's Telegram ID (numbers only):\n\nExample: 123456789",
		"set_limit_bad_user":         "❌ Invalid input. Please send only the Telegram ID (numbers).\n\nExample: 123456789",
		"set_limit_user_missing":     "❌ User not found with ID: {}\n\nMake sure the user has started the bot at least once.",
		"set_limit_success":          "✅ Limit updated successfully!\n\n👤 User: {}\n🆔 Username: {}\n🆔 ID: {}\n\n🔢 Daily Limit: {}\n📅 Valid Until: {}",
		"set_limit_failed":           "❌ Failed to update user limit. Please try again.",
		"unauthorized":               "❌ You are not authorized to use this command.",
		"cancelled":                  "❌ Set limit operation cancelled.",
		"open_in_browser":            "📥 Open In Browser",
		"support_label":              "🚀 Contact",
		"bot_label":                  "🤖 Bot",
		"copy_button":                "Copy",
		"share_button":               "Share",
		"choose_language_first":      "🌍 Please choose your language first:\nИлтимос, аввал тилни танланг\nПожалуйста, сначала выберите язык",
	},
	"uz": {
		"choose_language":            "🌍 Iltimos, tilingizni tanlang:\n\nPlease choose your language\nПожалуйста, выберите язык",
		"language_selected":          "✅ Til O'zbekcha qilib o'rnatildi",
		"welcome":                    "👋 Freepik Yuklab Olish Botiga Xush Kelibsiz!\n🆔 Sizning Telegram ID: {}\n\nHurmatli mijoz, sizda ilk bir hafta davomida har kuni 1 ta fayl yuklab olish imkoniyati mavjud. Agar ko'proq limit kerak bo'lsa admin bilan bog'laning.\nAdmin: @ProlinkAdmin\n\nBotga Freepik havolasini yuboring va bot sizga to'g'ridan-to'g'ri yuklab olish havolasini beradi.\n\nQo'llab-quvvatlanadigan turlar:\n✅ Ikonkalar, Vektorlar, Rasmlar\n✅ PSD Fayllar, Shablonlar\n✅ Mockuplar, Illustratsiyalar\n\nQo'shimcha ma'lumot uchun /help ni bosing.",
		"help":                       "ℹ️ Foydalanish yo'riqnomasi:\n\n1️⃣ Botga istalgan Freepik premium havolasini yuboring\n2️⃣ Bot ishlov berishini kuting (30-60 soniya)\n3️⃣ To'g'ridan-to'g'ri yuklab olish havolasini oling\n4️⃣ Faylni yuklab olish uchun havolani bosing\n\nBuyruqlar:\n/start - Botni ishga tushirish\n/language - Tilni o'zgartirish\n/help - Yordam xabarini ko'rsatish\n/check_limit - Limitlaringizni ko'rish",
		"help_admin":                 "ℹ️ Admin Yo'riqnomasi:\n\n1️⃣ Botga istalgan Freepik premium havolasini yuboring\n2️⃣ Bot ishlov berishini kuting (30-60 soniya)\n3️⃣ To'g'ridan-to'g'ri yuklab olish havolasini oling\n4️⃣ Faylni yuklab olish uchun havolani bosing\n\n👥 Foydalanuvchi Buyruqlari:\n/start - Botni ishga tushirish\n/language - Tilni o'zgartirish\n/help - Yordam xabarini ko'rsatish\n/check_limit - Limitlaringizni ko'rish\n\n🔑 Admin Buyruqlari:\n/set_limit - Foydalanuvchi limitini o'rnatish\n/cache_mode_on - Cache import rejimini yoqish\n/cache_mode_off - Cache import rejimini o'chirish\n/cache_mode_status - Cache navbatini ko'rish\n/cache_mode_clear - Cache navbatini tozalash\n/cancel - Aktiv admin jarayonini bekor qilish",
		"processing":                 "🔍 *Freepik havolangizga ishlov berilmoqda...*\n\n⏳ Bu 30-60 soniya vaqt olishi mumkin. Iltimos, kuting...",
		"error":                      "❌ Kechirasiz, yuklab olish havolasini ajratib olib bo'lmadi.\n\nLink to'g'riligini tekshiring. Agar muammo linkda bo'lmasa, adminga xabar bering.",
		"temporarily_unavailable":    "⏳ Hozircha xizmatda vaqtinchalik muammo bor.\n\nIltimos, birozdan keyin qayta urinib ko'ring.",
		"unsupported_type":           "⚠️ *Bu turdagi kontent hali qo'llab-quvvatlanmaydi*\n\nBot {} ni hozircha yuklab ololmaydi.\n\n💬 Agar bu fayl kerak bo'lsa, admin bilan bog'laning.",
		"no_url":                     "❌ Xabaringizda to'g'ri Freepik havolasi topilmadi.\n\nIltimos, to'g'ri Freepik havolasini yuboring.\nMisol: https://www.freepik.com/premium-psd/your-design_1234567.htm\n\nQo'shimcha ma'lumot uchun /help ni bosing.",
		"limit_reached":              "❌ *Kunlik Limit Tugadi*\n\nSiz kunlik yuklab olish limitingizga yetdingiz.\n\n📊 Bugun: {}/{}\n\nIltimos, ertaga qayta urinib ko'ring yoki ko'proq yuklab olish uchun admin bilan bog'laning.",
		"trial_expired":              "⚠️ *Sinov Muddati Tugadi*\n\nSizning 7 kunlik sinov muddatingiz tugadi.\n\n📊 Bugun: {}/{}\n\nIltimos, premium kirish uchun admin bilan bog'laning.",
		"check_limit_title":          "📊 Limitingiz:",
		"check_limit_name":           "👤 Ism",
		"check_limit_username":       "🆔 Foydalanuvchi nomi",
		"check_limit_stats":          "📅 Yuklab Olish Statistikasi",
		"check_limit_today":          "• Bugun",
		"check_limit_total":          "• Jami",
		"check_limit_status":         "Holat",
		"check_limit_premium":        "⭐ Premium",
		"check_limit_premium_until":  "📆 Premium muddati",
		"check_limit_daily_limit":    "♻️ Kunlik Limit",
		"check_limit_trial":          "🆓 Sinov Muddati",
		"check_limit_trial_ends":     "⏳ Sinov tugashi",
		"check_limit_days_left":      "kun qoldi",
		"check_limit_expired":        "⚠️ Sinov Muddati Tugadi",
		"check_limit_contact_admin":  "💬 Premium uchun admin @ProlinkAdmin bilan bog'laning",
		"check_limit_registered":     "📅 Ro'yxatdan o'tilgan",
		"check_limit_reset_info":     "🔄 Limitlar har kuni 00:00 da yangilanadi (Toshkent vaqti bilan)",
		"check_limit_not_registered": "❌ Siz hali ro'yxatdan o'tmagansiz. Ro'yxatdan o'tish uchun /start ni bosing.",
		"check_limit_error":          "❌ Limit ma'lumotini olishda xatolik. Iltimos keyinroq urinib ko'ring.",
		"set_limit_start":            "👑 Admin: Foydalanuvchi limitini o'rnatish\n\n📅 Limit tugash sanasini YYYY.MM.DD formatida yuboring\n\nMisol: 2026.12.31\n\nYoki muddatsiz qoldirish uchun 0 yuboring (faqat trial userlar uchun).",
		"set_limit_bad_date":         "❌ Sana formati noto'g'ri. Iltimos, YYYY.MM.DD formatidan foydalaning.\n\nMisol: 2026.12.31\nYoki muddatsiz qoldirish uchun 0 yuboring.",
		"set_limit_past_date":        "❌ Sana kelajakda bo'lishi kerak. Iltimos, qayta urinib ko'ring.\n\nFormat: YYYY.MM.DD (masalan, 2026.12.31)",
		"set_limit_ask_daily":        "✅ Sana saqlandi!\n\n🔢 Endi kunlik yuklab olish limitini yuboring (raqam):\n\nMisol: kuniga 10 ta yuklab olish uchun 10\nYoki 999 cheklanmagan uchun",
		"set_limit_bad_daily":        "❌ Noto'g'ri son. Iltimos, 10 yoki 999 kabi raqam yuboring.",
		"set_limit_ask_user":         "✅ Kunlik limit saqlandi!\n\n👤 Endi foydalanuvchining Telegram ID sini yuboring (faqat raqam):\n\nMisol: 123456789",
		"set_limit_bad_user":         "❌ Noto'g'ri kiritish. Iltimos, faqat Telegram ID ni yuboring.\n\nMisol: 123456789",
		"set_limit_user_missing":     "❌ Quyidagi ID bilan foydalanuvchi topilmadi: {}\n\nFoydalanuvchi hech bo'lmasa bir marta botni ishga tushirgan bo'lishi kerak.",
		"set_limit_success":          "✅ Limit muvaffaqiyatli yangilandi!\n\n👤 Foydalanuvchi: {}\n🆔 Username: {}\n🆔 ID: {}\n\n🔢 Kunlik limit: {}\n📅 Amal qilish muddati: {}",
		"set_limit_failed":           "❌ Foydalanuvchi limitini yangilab bo'lmadi. Qayta urinib ko'ring.",
		"unauthorized":               "❌ Siz bu buyruqdan foydalana olmaysiz.",
		"cancelled":                  "❌ Limit o'rnatish jarayoni bekor qilindi.",
		"open_in_browser":            "📥 Brauzerda Ochish",
		"support_label":              "🚀 Murojaat",
		"bot_label":                  "🤖 Bot",
		"copy_button":                "Copy",
		"share_button":               "Share",
		"choose_language_first":      "🌍 Iltimos, avval tilni tanlang:\nPlease choose your language\nПожалуйста, сначала выберите язык",
	},
	"ru": {
		"choose_language":            "🌍 Пожалуйста, выберите язык:\n\nPlease choose your language\nTilni tanlang",
		"language_selected":          "✅ Язык установлен на Русский",
		"welcome":                    "👋 Добро пожаловать в бот для загрузки Freepik!\n🆔 Ваш Telegram ID: {}\n\nУважаемый клиент, у вас есть возможность загружать 1 файл в день в течение первой недели. Если вам нужен больший лимит, свяжитесь с админом.\nАдмин: @ProlinkAdmin\n\nОтправьте ссылку Freepik боту, и он предоставит вам прямую ссылку для скачивания.\n\nПоддерживаемые типы:\n✅ Иконки, Векторы, Фото\n✅ PSD Файлы, Шаблоны\n✅ Макеты, Иллюстрации\n\nДля получения дополнительной информации нажмите /help.",
		"help":                       "ℹ️ Руководство пользователя:\n\n1️⃣ Отправьте боту любую премиум ссылку Freepik\n2️⃣ Подождите, пока бот обработает её (30-60 секунд)\n3️⃣ Получите прямую ссылку для скачивания\n4️⃣ Нажмите на ссылку, чтобы скачать файл\n\nКоманды:\n/start - Запустить бота\n/language - Изменить язык\n/help - Показать справку\n/check_limit - Просмотреть лимиты",
		"help_admin":                 "ℹ️ Руководство администратора:\n\n1️⃣ Отправьте боту любую премиум ссылку Freepik\n2️⃣ Подождите, пока бот обработает её (30-60 секунд)\n3️⃣ Получите прямую ссылку для скачивания\n4️⃣ Нажмите на ссылку, чтобы скачать файл\n\n👥 Команды пользователя:\n/start - Запустить бота\n/language - Изменить язык\n/help - Показать справку\n/check_limit - Просмотреть лимиты\n\n🔑 Команды администратора:\n/set_limit - Установить лимит пользователю\n/cache_mode_on - Включить cache import режим\n/cache_mode_off - Выключить cache import режим\n/cache_mode_status - Показать очередь cache\n/cache_mode_clear - Очистить очередь cache\n/cancel - Отменить активный admin flow",
		"processing":                 "🔍 *Обрабатываю вашу ссылку Freepik...*\n\n⏳ Это может занять 30-60 секунд. Пожалуйста, подождите...",
		"error":                      "❌ Извините, не удалось извлечь ссылку для скачивания.\n\nПроверьте правильность ссылки. Если проблема не в ссылке, свяжитесь с админом.",
		"temporarily_unavailable":    "⏳ Сервис временно недоступен.\n\nПожалуйста, попробуйте немного позже.",
		"unsupported_type":           "⚠️ *Этот тип контента пока не поддерживается*\n\nБот не может загрузить {} в данный момент.\n\n💬 Если вам нужен этот файл, свяжитесь с админом.",
		"no_url":                     "❌ В вашем сообщении не найдена действительная ссылка Freepik.\n\nПожалуйста, отправьте действительную ссылку Freepik.\nПример: https://www.freepik.com/premium-psd/your-design_1234567.htm\n\nВведите /help для дополнительной информации.",
		"limit_reached":              "❌ *Дневной Лимит Исчерпан*\n\nВы достигли дневного лимита загрузок.\n\n📊 Сегодня: {}/{}\n\nПожалуйста, попробуйте завтра или свяжитесь с админом.",
		"trial_expired":              "⚠️ *Пробный Период Истек*\n\nВаш 7-дневный пробный период закончился.\n\n📊 Сегодня: {}/{}\n\nПожалуйста, свяжитесь с админом для премиум доступа.",
		"check_limit_title":          "📊 Ваш Лимит:",
		"check_limit_name":           "👤 Имя",
		"check_limit_username":       "🆔 Имя пользователя",
		"check_limit_stats":          "📅 Статистика Загрузок",
		"check_limit_today":          "• Сегодня",
		"check_limit_total":          "• Всего",
		"check_limit_status":         "Статус",
		"check_limit_premium":        "⭐ Premium",
		"check_limit_premium_until":  "📆 Premium до",
		"check_limit_daily_limit":    "♻️ Дневной Лимит",
		"check_limit_trial":          "🆓 Пробный Период",
		"check_limit_trial_ends":     "⏳ Пробный период заканчивается",
		"check_limit_days_left":      "дней осталось",
		"check_limit_expired":        "⚠️ Пробный Период Истек",
		"check_limit_contact_admin":  "💬 Свяжитесь с админом @ProlinkAdmin для премиум доступа",
		"check_limit_registered":     "📅 Зарегистрирован",
		"check_limit_reset_info":     "🔄 Лимиты сбрасываются ежедневно в 00:00 (Ташкент)",
		"check_limit_not_registered": "❌ Вы еще не зарегистрированы. Используйте /start для регистрации.",
		"check_limit_error":          "❌ Ошибка получения информации о лимите. Пожалуйста, попробуйте позже.",
		"set_limit_start":            "👑 Admin: Установка лимита пользователю\n\n📅 Отправьте дату окончания лимита в формате YYYY.MM.DD\n\nПример: 2026.12.31\n\nИли отправьте 0 для отсутствия даты окончания (только для trial users).",
		"set_limit_bad_date":         "❌ Неверный формат даты. Используйте формат YYYY.MM.DD\n\nПример: 2026.12.31\nИли отправьте 0 для отсутствия даты окончания.",
		"set_limit_past_date":        "❌ Дата должна быть в будущем. Попробуйте снова.\n\nФормат: YYYY.MM.DD (например, 2026.12.31)",
		"set_limit_ask_daily":        "✅ Дата сохранена!\n\n🔢 Теперь отправьте дневной лимит загрузок (число):\n\nПример: 10 для 10 загрузок в день\nИли 999 для безлимита",
		"set_limit_bad_daily":        "❌ Неверное число. Пожалуйста, отправьте число вроде 10 или 999.",
		"set_limit_ask_user":         "✅ Дневной лимит сохранен!\n\n👤 Теперь отправьте Telegram ID пользователя (только цифры):\n\nПример: 123456789",
		"set_limit_bad_user":         "❌ Неверный ввод. Пожалуйста, отправьте только Telegram ID (цифры).\n\nПример: 123456789",
		"set_limit_user_missing":     "❌ Пользователь с ID {} не найден.\n\nУбедитесь, что пользователь хотя бы один раз запустил бота.",
		"set_limit_success":          "✅ Лимит успешно обновлен!\n\n👤 Пользователь: {}\n🆔 Username: {}\n🆔 ID: {}\n\n🔢 Дневной лимит: {}\n📅 Действует до: {}",
		"set_limit_failed":           "❌ Не удалось обновить лимит пользователя. Попробуйте снова.",
		"unauthorized":               "❌ Вы не авторизованы для использования этой команды.",
		"cancelled":                  "❌ Операция установки лимита отменена.",
		"open_in_browser":            "📥 Открыть в браузере",
		"support_label":              "🚀 Обращение",
		"bot_label":                  "🤖 Bot",
		"copy_button":                "Copy",
		"share_button":               "Share",
		"choose_language_first":      "🌍 Пожалуйста, сначала выберите язык:\nPlease choose your language\nIltimos, avval tilni tanlang",
	},
}

func NormalizeLang(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	switch {
	case strings.HasPrefix(code, "uz"):
		return "uz"
	case strings.HasPrefix(code, "ru"):
		return "ru"
	case strings.HasPrefix(code, "en"):
		return "en"
	default:
		return "en"
	}
}

func GetText(lang, key string) string {
	lang = NormalizeLang(lang)
	if langMap, ok := translations[lang]; ok {
		if msg, ok := langMap[key]; ok {
			return msg
		}
	}
	if msg, ok := translations["en"][key]; ok {
		return msg
	}
	return key
}

func BuildLimitMessage(lang, errorMsg string, downloadsToday, dailyLimit int) string {
	key := "limit_reached"
	if strings.Contains(strings.ToLower(errorMsg), "trial") {
		key = "trial_expired"
	}
	return ReplacePlaceholders(GetText(lang, key), strconv.Itoa(downloadsToday), strconv.Itoa(dailyLimit))
}

func BuildLimitOnlyText(lang string, downloadsToday, dailyLimit int) string {
	lang = NormalizeLang(lang)
	switch lang {
	case "uz":
		return "📊 Bugun: " + strconv.Itoa(downloadsToday) + "/" + strconv.Itoa(dailyLimit)
	case "ru":
		return "📊 Сегодня: " + strconv.Itoa(downloadsToday) + "/" + strconv.Itoa(dailyLimit)
	default:
		return "📊 Today: " + strconv.Itoa(downloadsToday) + "/" + strconv.Itoa(dailyLimit)
	}
}

func BuildSuccessMessage(lang, downloadLink, limitText, supportURL, botURL string) string {
	return strings.Join([]string{
		downloadLink,
		limitText,
		"",
		GetText(lang, "support_label") + " || " + GetText(lang, "bot_label"),
		supportURL + " || " + botURL,
	}, "\n")
}

func BuildSuccessMessageHTML(lang, downloadLink, limitText, supportURL, botURL string) string {
	return strings.Join([]string{
		html.EscapeString(downloadLink),
		html.EscapeString(limitText),
		"",
		`<a href="` + html.EscapeString(supportURL) + `">` + html.EscapeString(GetText(lang, "support_label")) + `</a> || <a href="` + html.EscapeString(botURL) + `">` + html.EscapeString(GetText(lang, "bot_label")) + `</a>`,
	}, "\n")
}

func BuildVideoDeliveredMessageHTML(lang, limitText, supportURL, botURL string) string {
	return strings.Join([]string{
		html.EscapeString(videoDeliveredLabel(lang)),
		html.EscapeString(limitText),
		"",
		`<a href="` + html.EscapeString(supportURL) + `">` + html.EscapeString(GetText(lang, "support_label")) + `</a> || <a href="` + html.EscapeString(botURL) + `">` + html.EscapeString(GetText(lang, "bot_label")) + `</a>`,
	}, "\n")
}

func BuildCachedDeliveredMessageHTML(lang, limitText, supportURL, botURL string) string {
	return strings.Join([]string{
		html.EscapeString(limitText),
		"",
		`<a href="` + html.EscapeString(supportURL) + `">` + html.EscapeString(GetText(lang, "support_label")) + `</a> || <a href="` + html.EscapeString(botURL) + `">` + html.EscapeString(GetText(lang, "bot_label")) + `</a>`,
	}, "\n")
}

func videoDeliveredLabel(lang string) string {
	switch NormalizeLang(lang) {
	case "uz":
		return "🎬 Video fayl quyida yuborildi"
	case "ru":
		return "🎬 Видео отправлено ниже"
	default:
		return "🎬 Video file sent below"
	}
}

func ReplacePlaceholders(text string, values ...string) string {
	for _, value := range values {
		text = strings.Replace(text, "{}", value, 1)
	}
	return text
}
