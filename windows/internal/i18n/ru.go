package i18n

// Русский.
var russian = [numKeys]string{

	// Tray menu: header, state, modes
	MenuLayout:       "Раскладка: %s",
	MenuHooksRunning: "Перехват клавиатуры: работает",
	MenuHooksStopped: "⚠ Перехват клавиатуры: остановлен",
	MenuEnabled:      "Включено",
	MenuSmartMode:    "Умный режим (распознавать автоматически)",
	MenuDefaultMode:  "Режим по умолчанию",
	ModeAuto:         "Автоисправление",
	ModeSuggest:      "Только подсказки",
	ModeOff:          "Выключено",

	// Sensitivity
	MenuSensitivity:            "Насколько смело исправлять: %s",
	MenuSensitivityCustom:      "Сейчас: своя (%.2f / %.2f) — %s",
	SensitivityCautious:        "Осторожно",
	SensitivityBalanced:        "Сбалансировано",
	SensitivityBold:            "Смело",
	SensitivityCustom:          "Своя",
	SensitivityHintCautious:    "сам исправляет только явные ошибки, остальное предлагает",
	SensitivityHintBalanced:    "явные ошибки исправляет сам, сомнительные предлагает",
	SensitivityHintBold:        "сам исправляет почти всё, что похоже на ошибку",
	SensitivityExplainCautious: "Сам исправляет только явные ошибки, остальное предлагает. Меньше всего неожиданных замен.",
	SensitivityExplainBalanced: "Явные ошибки исправляет сам, сомнительные — предлагает. Подходит большинству.",
	SensitivityExplainBold:     "Сам исправляет почти всё, что похоже на ошибку. Больше помощи — но иногда и лишние замены.",
	SensitivityExplainCustom:   "Пороги заданы вручную в config.json (autoThreshold, suggestThreshold).",

	// Languages
	MenuLanguages:      "Языки",
	MenuLayoutMissing:  "%s — раскладка не добавлена в Windows",
	MenuCyrillicHeader: "Русский и казахский — когда по буквам не отличить:",
	CyrillicAutomatic:  "Авто + память по приложениям",
	CyrillicKazakh:     "Приоритет казахскому",
	CyrillicRussian:    "Приоритет русскому",

	// Learning and stored words
	MenuLearning:           "Обучение",
	MenuLearningToggle:     "Учиться на моих действиях",
	MenuLearningCounts1:    "Не исправлять: %d · Исправлять без вопроса: %d",
	MenuLearningCounts2:    "Мои слова: %d · Наблюдаю: %d",
	MenuLearningReset:      "Забыть всё выученное…",
	MenuClearExceptions:    "Очистить все слова-исключения…",
	ConfirmResetLearned:    "Забыть всё, чему Tilqos научился на ваших действиях?\n\nСлова-исключения из настроек останутся.",
	ConfirmClearExceptions: "Очистить все слова-исключения?\n\nTilqos снова будет исправлять эти слова. Выученные слова останутся как есть.",

	// Per-app items
	MenuAppAlwaysOff: "«%s» — всегда выключено",
	MenuAppOff:       "Не исправлять в «%s»",
	MenuAppSuggest:   "Только подсказки в «%s»",

	// Actions (menu rows and the hotkeys list)
	ActFixLastWord:      "Исправить последнее слово",
	ActFixSelection:     "Исправить выделение",
	ActUndoLastFix:      "Отменить последнее исправление",
	ActSwitchTo:         "Переключить на %s",
	ActConvertClipboard: "Конвертировать буфер обмена",
	ActTransliterate:    "Транслитерация (латиница ⇄ кириллица)",
	ActToggleEnabled:    "Включить / выключить",

	// Transliteration submenu
	MenuTransliteration:    "Транслитерация",
	MenuTranslitAuto:       "Определить направление",
	MenuTranslitToCyrillic: "Латиница → Кириллица",
	MenuTranslitToLatin:    "Кириллица → Латиница",
	MenuTranslitScheme:     "Схема латиницы",
	SchemeOfficial:         "Официальная латиница 2021",
	SchemePractical:        "Практическая (ASCII)",
	SchemeOfficialDetail:   "31 буква с диакритикой: ä ğ ñ ö ū ü ş. Обратимо один к одному.",
	SchemePracticalDetail:  "Без диакритики, как пишут в чатах. Обратно — с языковой моделью.",

	// Bottom of the menu
	MenuSwitchLayout:      "Переключить раскладку",
	MenuAutostart:         "Запускать при входе в Windows",
	MenuOpenFolder:        "Открыть папку настроек и журнала",
	MenuHotkeys:           "Горячие клавиши…",
	MenuLog:               "Журнал",
	MenuLogEnabled:        "Вести журнал (по умолчанию выключено)",
	MenuLogClear:          "Удалить файл журнала",
	MsgLogClearFailed:     "Не удалось удалить файл журнала: %s",
	MenuInterfaceLanguage: "Язык интерфейса",
	LanguageSystem:        "Как в системе",
	MenuAbout:             "О программе",
	MenuAboutVersion:      "Tilqos %s",
	MenuPrivacy:           "Приватность…",
	MenuLicenses:          "Лицензии…",
	MenuQuit:              "Выход",
	TrayEnabled:           "Tilqos — включён",
	TrayDisabled:          "Tilqos — выключен",

	// Dialogs and notifications
	MsgAlreadyRunning:          "Tilqos уже запущен — его значок в области уведомлений.",
	MsgModelsMissing:           "Языковые модели не встроены в эту сборку:\n\n%s\n\nСоберите программу через build.ps1 или build.sh.",
	MsgWindowFailed:            "Не удалось создать окно: %s",
	MsgAutostartFailed:         "Не удалось изменить автозапуск: %s",
	BalloonHookTitle:           "Перехват клавиатуры не работает",
	BalloonHookText:            "Не удалось установить перехват клавиатуры: %s",
	BalloonSettingsReset:       "Настройки сброшены",
	BalloonWelcomeTitle:        "Tilqos работает",
	BalloonWelcomeText:         "Исправляю текст, набранный не в той раскладке. %s — исправить слово, %s — выделение, %s — отмена. Меню — по значку.",
	NotifyUndoUnavailableTitle: "Отмена недоступна",
	NotifyUndoUnavailableText:  "После исправления текст уже изменился — отменить его вслепую небезопасно.",

	// Hotkeys window
	HelpTitle:     "Горячие клавиши Tilqos",
	HelpAccept:    "%s — принять подсказку (в терминалах и редакторах — Insert).",
	HelpUndoAgain: "%s сразу после исправления — отменить его.",
	HelpFooter:    "Клавиши меняются в %s\n(раздел \"hotkeys\", например \"Ctrl+Alt+Z\").",

	// Privacy and licenses
	PrivacyText:     "Tilqos следит за нажатыми клавишами, чтобы заметить неверную раскладку. Вот что он с ними делает:\n\n• Нет доступа к сети, нет телеметрии, нет проверки обновлений.\n• Набранное хранится только в памяти. Оно очищается не позднее чем через 5 секунд без набора, при смене приложения или окна, на Enter, Tab, Esc и подобных клавишах, в полях для паролей и при блокировке компьютера или его сне.\n• «Отмена» хранит последние исправления в памяти не дольше 2 минут и стирает их при смене приложения или окна, блокировке или сне компьютера и выходе.\n• Буфер обмена используется только действиями, которым он нужен, а его прежнее содержимое возвращается. Скрытое или временное содержимое, например из менеджеров паролей, никогда не читается: действие молча ничего не делает. Текст длиннее 10 000 символов игнорируется.\n• Набранный текст, содержимое буфера обмена и заголовки окон никогда не пишутся в журнал. Журнал по умолчанию выключен; если его включить, в нём длины слов, языки и решения, в папке ниже.\n• В %s хранятся: настройки, правила для приложений, добавленные вами слова-исключения (обычным текстом) и выученные слова (хеши с солью, 64 бита; это не шифрование: тот, кто может прочитать файл, способен угадать короткие распространённые слова).\n• Windows узнаёт только стандартные поля для паролей. Поля для паролей в браузерах и в нестандартных элементах могут не распознаваться.",
	LicensesMissing: "Тексты лицензий не входят в эту сборку. Они есть в репозитории проекта.",
	LicensesFailed:  "Не удалось открыть тексты лицензий: %s",
}
