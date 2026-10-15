package i18n

// Қазақша (Cyrillic). NEEDS NATIVE REVIEW.
var kazakh = [numKeys]string{

	// Tray menu: header, state, modes
	MenuLayout:       "Орналасым: %s",
	MenuHooksRunning: "Пернетақтаны бақылау: жұмыс істеп тұр",
	MenuHooksStopped: "⚠ Пернетақтаны бақылау: тоқтатылған",
	MenuEnabled:      "Қосулы",
	MenuSmartMode:    "Ақылды режим (автоматты түрде тану)",
	MenuDefaultMode:  "Әдепкі режим",
	ModeAuto:         "Автотүзету",
	ModeSuggest:      "Тек ұсыныстар",
	ModeOff:          "Өшірулі",

	// Sensitivity
	MenuSensitivity:            "Қаншалықты батыл түзету: %s",
	MenuSensitivityCustom:      "Қазір: жеке (%.2f / %.2f) — %s",
	SensitivityCautious:        "Сақ",
	SensitivityBalanced:        "Теңгерімді",
	SensitivityBold:            "Батыл",
	SensitivityCustom:          "Жеке",
	SensitivityHintCautious:    "тек айқын қателерді өзі түзетеді, қалғанын ұсынады",
	SensitivityHintBalanced:    "айқын қателерді өзі түзетеді, күмәнділерін ұсынады",
	SensitivityHintBold:        "қателікке ұқсайтын дерлік бәрін өзі түзетеді",
	SensitivityExplainCautious: "Тек айқын қателерді өзі түзетеді, қалғанын ұсынады. Күтпеген ауыстырулар ең аз.",
	SensitivityExplainBalanced: "Айқын қателерді өзі түзетеді, күмәнділерін ұсынады. Көпшілікке жарайды.",
	SensitivityExplainBold:     "Қателікке ұқсайтын дерлік бәрін өзі түзетеді. Көмек көбірек, бірақ кейде артық ауыстыру болады.",
	SensitivityExplainCustom:   "Шектер config.json файлында қолмен берілген (autoThreshold, suggestThreshold).",

	// Languages
	MenuLanguages:      "Тілдер",
	MenuLayoutMissing:  "%s — орналасым Windows жүйесіне қосылмаған",
	MenuCyrillicHeader: "Орыс және қазақ — әріптерден айыруға болмаса:",
	CyrillicAutomatic:  "Авто + қолданбаға байлау",
	CyrillicKazakh:     "Қазақ тіліне басымдық",
	CyrillicRussian:    "Орыс тіліне басымдық",

	// Learning and stored words
	MenuLearning:           "Үйрену",
	MenuLearningToggle:     "Менің әрекеттерімнен үйрену",
	MenuLearningCounts1:    "Енді түзетпеу: %d · Сұрамай түзету: %d",
	MenuLearningCounts2:    "Сіздің сөздеріңіз: %d · Бақылауда: %d",
	MenuLearningReset:      "Үйренгеннің бәрін ұмыту…",
	MenuClearExceptions:    "Барлық ерекшелік сөздерді тазалау…",
	ConfirmResetLearned:    "Tilqos сіздің әрекеттеріңізден үйренгеннің бәрі ұмытылсын ба?\n\nБаптаулардағы ерекшелік сөздер қалады.",
	ConfirmClearExceptions: "Барлық ерекшелік сөздер тазалансын ба?\n\nTilqos бұл сөздерді қайтадан түзететін болады. Үйренген сөздер өзгеріссіз қалады.",

	// Per-app items
	MenuAppAlwaysOff: "«%s» — әрқашан өшірулі",
	MenuAppOff:       "«%s» ішінде түзетпеу",
	MenuAppSuggest:   "«%s» ішінде тек ұсыныстар",

	// Actions (menu rows and the hotkeys list)
	ActFixLastWord:      "Соңғы сөзді түзету",
	ActFixSelection:     "Белгіленген мәтінді түзету",
	ActUndoLastFix:      "Соңғы түзетуді болдырмау",
	ActSwitchTo:         "%s тіліне ауыстыру",
	ActConvertClipboard: "Алмасу буферін түрлендіру",
	ActTransliterate:    "Транслитерация (латын ⇄ кирилл)",
	ActToggleEnabled:    "Қосу / өшіру",

	// Transliteration submenu
	MenuTransliteration:    "Транслитерация",
	MenuTranslitAuto:       "Бағытты анықтау",
	MenuTranslitToCyrillic: "Латын → Кирилл",
	MenuTranslitToLatin:    "Кирилл → Латын",
	MenuTranslitScheme:     "Латын әліпбиі нұсқасы",
	SchemeOfficial:         "Ресми латын әліпбиі 2021",
	SchemePractical:        "Практикалық (ASCII)",
	SchemeOfficialDetail:   "Диакритикалы 31 әріп: ä ğ ñ ö ū ü ş. Бір әріпке бір әріптен сәйкес, толық қайтымды.",
	SchemePracticalDetail:  "Диакритикасыз, чаттарда жазатындай. Кері аудару тіл моделі арқылы жасалады.",

	// Bottom of the menu
	MenuSwitchLayout:      "Орналасымды ауыстыру",
	MenuAutostart:         "Windows-қа кіргенде іске қосу",
	MenuOpenFolder:        "Баптаулар мен журнал қалтасын ашу",
	MenuHotkeys:           "Жылдам пернелер…",
	MenuLog:               "Журнал",
	MenuLogEnabled:        "Журнал жүргізу (әдепкіде өшірулі)",
	MenuLogClear:          "Журнал файлын жою",
	MsgLogClearFailed:     "Журнал файлын жою мүмкін болмады: %s",
	MenuInterfaceLanguage: "Интерфейс тілі",
	LanguageSystem:        "Жүйедегідей",
	MenuAbout:             "Туралы",
	MenuAboutVersion:      "Tilqos %s",
	MenuPrivacy:           "Құпиялылық…",
	MenuLicenses:          "Лицензиялар…",
	MenuQuit:              "Шығу",
	TrayEnabled:           "Tilqos — қосулы",
	TrayDisabled:          "Tilqos — өшірулі",

	// Dialogs and notifications
	MsgAlreadyRunning:          "Tilqos іске қосылып қойған — оның белгішесі хабарландыру аймағында.",
	MsgModelsMissing:           "Тіл модельдері бұл құрастырмаға енгізілмеген:\n\n%s\n\nБағдарламаны build.ps1 немесе build.sh арқылы құрастырыңыз.",
	MsgWindowFailed:            "Терезені жасау мүмкін болмады: %s",
	MsgAutostartFailed:         "Автоқосуды өзгерту мүмкін болмады: %s",
	BalloonHookTitle:           "Пернетақтаны бақылау жұмыс істемейді",
	BalloonHookText:            "Пернетақтаны бақылауды іске қосу мүмкін болмады: %s",
	BalloonSettingsReset:       "Баптаулар қалпына келтірілді",
	BalloonWelcomeTitle:        "Tilqos жұмыс істеп тұр",
	BalloonWelcomeText:         "Қате орналасымда терілген мәтінді түзетемін. %s — сөзді түзету, %s — белгіленген мәтін, %s — болдырмау. Мәзір — белгішеде.",
	NotifyUndoUnavailableTitle: "Болдырмау мүмкін емес",
	NotifyUndoUnavailableText:  "Түзетуден кейін мәтін өзгерді, сондықтан оны байқамай болдырмау қауіпті.",

	// Hotkeys window
	HelpTitle:     "Tilqos жылдам пернелері",
	HelpAccept:    "%s — ұсынысты қабылдау (терминалдар мен редакторларда — Insert).",
	HelpUndoAgain: "Түзетуден кейін бірден %s — оны болдырмау.",
	HelpFooter:    "Пернелер %s файлында өзгертіледі\n(\"hotkeys\" бөлімі, мысалы, \"Ctrl+Alt+Z\").",

	// Privacy and licenses
	PrivacyText:     "Tilqos қате орналасымды байқау үшін басылған пернелерді бақылайды. Олармен не істейтіні мынау:\n\n• Желіге қолжетімділік жоқ, телеметрия жоқ, жаңартуларды тексеру жоқ.\n• Терілгендер тек жадта сақталады. Ол теру тоқтағаннан кейін ең көбі 5 секундта, қолданбаны немесе терезені ауыстырғанда, Enter, Tab, Esc және соған ұқсас пернелерде, құпиясөз өрістерінде, сондай-ақ компьютер құлыпталғанда немесе ұйқы режиміне өткенде тазаланады.\n• «Болдырмау» соңғы түзетулерді жадта 2 минуттан артық сақтамайды, ал қолданбаны немесе терезені ауыстырғанда, компьютер құлыпталғанда немесе ұйықтағанда, шыққанда оларды өшіреді.\n• Алмасу буфері тек оны қажет ететін әрекеттерде пайдаланылады, ал оның бұрынғы мазмұны қайтарылады. Жасырын немесе уақытша мазмұн (мысалы, құпиясөз менеджерлерінен) ешқашан оқылмайды: әрекет үнсіз ештеңе істемейді. 10 000 таңбадан ұзын мәтін еленбейді.\n• Терілген мәтін, алмасу буферінің мазмұны және терезе тақырыптары журналға ешқашан жазылмайды. Журнал әдепкіде өшірулі; қосылса, онда сөздердің ұзындығы, тілдер және шешімдер төмендегі қалтада сақталады.\n• %s қалтасында сақталатындар: баптаулар, қолданба ережелері, өзіңіз қосқан ерекшелік сөздер (қарапайым мәтін) және үйренген сөздер (тұздалған 64 биттік хэштер; бұл шифрлау емес: файлды оқи алатын адам қысқа, жиі кездесетін сөздерді болжап таба алады).\n• Windows тек стандартты құпиясөз өрістерін таниды. Браузерлердегі және өзгеше салынған элементтердегі құпиясөз өрістері анықталмауы мүмкін.",
	LicensesMissing: "Лицензия мәтіндері бұл құрастырмаға кірмеген. Олар жобаның репозиториінде бар.",
	LicensesFailed:  "Лицензия мәтіндерін ашу мүмкін болмады: %s",
}
