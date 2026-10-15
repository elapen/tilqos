import Foundation

@testable import TilqosKit

/// The regression corpus the detector is measured against.
///
/// Written by hand rather than sampled, because what matters is what people actually type: the
/// few hundred words that make up half of all running text, the sentences they appear in, and
/// the ways a wrong layout mangles them. Two directions are measured separately, since they
/// fail differently:
///
/// - **must stay** — correctly typed text. Any change here is a false positive, the failure users
///   uninstall apps over (`еще` → `tot`).
/// - **must convert** — text typed on the wrong layout. A miss here is a false negative, a
///   shrug: the user presses ⌥⇧F.
enum Corpus {
    private static func words(_ text: String) -> [String] {
        text.split(whereSeparator: \.isWhitespace).map(String.init)
    }

    // MARK: - Frequent words that must never change

    /// The most frequent Russian words, with the common inflected forms — the lemma is often not
    /// what gets typed (`сделал`, `людей`, `мне`).
    static let russianFrequent = words(
        """
        и в не на я быть он с что а по это она этот к но они мы как из у который то за свой весь
        год от так о для ты же все тот мочь вы человек такой его сказать только или еще ещё бы
        себя один когда уже до время если сам нет другой вот говорить наш мой знать стать при
        чтобы дело жизнь кто первый очень два день ее её новый рука даже во со раз где там под
        можно ну какой после их работа без самый потом надо хотеть ли слово идти большой должен
        место иметь ничто сейчас тут лицо каждый друг нельзя теперь ни глаз тоже тогда видеть
        вопрос через да здесь дом потому сторона думать сделать страна жить чем мир об последний
        случай голова более делать смотреть ребенок просто конечно сила конец перед несколько час
        понять пойти город сразу земля вода книга дверь машина деньги спасибо пожалуйста привет
        хорошо ладно давай сегодня завтра вчера всегда никогда иногда почему зачем куда откуда
        сколько много мало больше меньше лучше хуже правда наверное кстати например значит вроде
        типа короче блин мне меня тебя тебе ему ей нам нас вас вам им них ним него нее неё этого
        этом этой эти этих того том той те тех всё всего всех всем был была было были будет буду
        будем будут есть говорит говорил сказал сказала знаю знаешь знает может могу можешь хочу
        хочешь хотел хотела нужно пока сделал сделала сделали сделаю пошел пошла пришел пришла
        видел видела думаю думаешь понимаю понял поняла работаю работает живу живет люди людей
        человека дома делаю делает смотрю вижу слышу хорошая хороший хорошие новая новые новое
        первая большая большие своей своих своим моей моих мои моя мое наша наши ваш ваша ваши
        которая которые которых году года лет дня дней раза разве также поэтому хотя пусть либо
        три четыре пять десять сто тысяча рубль рублей минута минут неделя месяц утро вечер ночь
        мама папа сын дочь брат сестра жена муж школа улица окно стол телефон письмо сообщение
        новости игра фильм музыка еда чай кофе магазин работу работы задача задачи проект сайт
        ответ вопросы идея план текст файл папка ссылка пароль экран кнопка ошибка программа
        понятно отлично хорошего спокойной ночи утра удачи конечно наконец сначала вместе опять
        снова вообще совсем почти сильно быстро медленно рано поздно давно недавно скоро куда-то
        кто-то что-то где-то тот тут там так как что это еще уже мне нет все его она они кто где
        чем тем том той ней нем них вас нас вам нам вот мой моя мое наш ваш сам раз два три дом
        мир год час бог мяч лес сад сон нос рот лук сыр сок чай суп торт хлеб рыба мясо вино пиво
        """)

    static let englishFrequent = words(
        """
        the be to of and a in that have i it for not on with he as you do at this but his by from
        they we say her she or an will my one all would there their what so up out if about who
        get which go me when make can like time no just him know take people into year your good
        some could them see other than then now look only come its over think also back after use
        two how our work first well way even new want because any these give day most us is was
        are were been has had did does said got made went going done top yes ok okay hey hi thanks
        thank please sorry here where why very much more many such must should may might let put
        set run own old big small long great little right left same next last few high low end
        part place case week company system program question number night point home water room
        mother area money story fact month lot study book eye job word business issue side kind
        head house service friend father power hour game line member law car city name team minute
        idea kid body information parent face others level office door health person art war
        history party result change morning reason research girl guy moment air teacher force
        education foot boy age policy everything process music market sense nation plan college
        interest death experience effect class control care field role effort rate heart show
        leader light voice wife police mind price report decision son view town road arm
        difference value building action model season society tax director position player record
        paper space ground form event official matter center couple site project activity star
        table need court oil situation cost industry figure street image phone data picture
        practice piece land product doctor wall patient worker news test movie north love support
        technology step baby computer type attention film tree source organization hair window
        evidence population truth song am can't don't won't i'm it's that's there's what's let's
        i'll you're we're they're isn't aren't wasn't didn't doesn't couldn't wouldn't shouldn't
        going doing getting making taking looking working thinking coming trying saying used called
        asked seemed looked wanted needed started worked played lived loved moved told found felt
        left kept brought began became happened email online website app meeting today tomorrow
        yesterday tonight maybe really actually probably already still again always never often
        sometimes soon later early late fast slow hard easy nice cool fine sure true false open
        close start stop help call send read write speak listen watch play buy sell pay eat drink
        sleep walk talk ask answer learn teach fix check update file folder link password screen
        button error bug code build release version feature issue team chat message video photo
        """)

    static let kazakhFrequent = words(
        """
        мен сен ол біз сіз олар бұл сол осы анау мынау және бар жоқ емес еді екен деп үшін туралы
        бойынша арқылы қалай неге неліктен қайда қашан кім не қандай қанша өте көп аз жақсы жаман
        үлкен кіші жаңа ескі бір екі үш төрт бес алты жеті сегіз тоғыз он жиырма отыз қырық елу
        жүз мың күн түн ай жыл апта уақыт адам бала ана әке аға апа іні қыз ұл дос үй су нан ет
        шай сүт сәлем сәлеметсіз рахмет рақмет кешіріңіз иә болады болды болса болып керек мүмкін
        біледі білемін білмеймін айтты айту келді келу барды бару көрдім алды берді жазу оқу
        сөйлеу тұр отыр жүр қара іс жұмыс мектеп қала ауыл ел тіл қазақ орыс қазақша орысша
        ағылшынша бүгін ертең кеше қазір енді тағы да де та те ғой ғана қана ма ме ба бе па пе ше
        сонда сондықтан бірақ егер әлде немесе әрі тек барлық әр өз өзі менің сенің оның біздің
        сіздің олардың маған саған оған бізге сізге оларға мені сені оны бізді сізді оларды жерде
        жер көк ақ қызыл сары жасыл ыстық суық жылы кітап сөз сұрақ жауап әңгіме өмір ойын ән әлем
        халық мемлекет қоғам білім ғылым тарих мәдениет денсаулық дәрігер мұғалім оқушы студент
        """)

    // MARK: - Words typed on the wrong layout, which must be converted

    /// Russian words the user types while an English layout is active. Includes the words whose
    /// keys are punctuation on ABC — `было` comes out as `,skj`, `хорошо` as `[jhjij`.
    static let russianOnEnglish = words(
        """
        привет тест как дела спасибо пожалуйста хорошо сегодня работа вопрос человек компьютер
        клавиатура переключатель программа сообщение документ письмо встреча завтра вечером
        понедельник нет тут там вот мне все что так где кто когда почему сейчас потом можно нужно
        надо знаю думаю хочу могу было тебе люблю тоже уже жизнь объект съел эти этот ещё очень
        только может будет была были буду делать сделать сказать говорить работать смотреть
        слушать читать писать звонить понимаю отлично конечно давай ладно пока здравствуйте
        извините проблема решение ответ интересно быстро вместе вообще кажется наверное
        """)

    /// English words typed while a Cyrillic layout is active.
    static let englishOnRussian = words(
        """
        hello world keyboard test please thanks what where when which there their about would
        could should people because really right think know good great time work home the and you
        not but for are was his they have this with from that your will one all can out get has
        him how its may new now old see two way who boy did let put say she too use don't i'm it's
        meeting project update release version feature computer message question problem answer
        """)

    /// Kazakh words that contain a Kazakh-only letter, so the Russian layout turns them into
    /// digits (`сәлеметсіз` → `с2леметс3з`). Kazakh words spelled only with Russian letters look
    /// identical on both layouts and are not a mistake at all.
    static let kazakhOnRussian = words(
        """
        сәлеметсіз қазақ жақсы үшін бұл және көп өте сәлем қалай рақмет кешіріңіз біз сіз менің
        сенің оның біздің сіздің маған саған оған бізге сізге қазақша ағылшынша әлем халық ғылым
        мәдениет денсаулық дәрігер мұғалім оқушы тіл жұмыс түн күн жүз мың тоғыз төрт сегіз
        бүгін ертең қазір мүмкін ғой ғана өмір әңгіме көк қызыл
        """)

    /// Kazakh words typed while an English layout is active.
    static let kazakhOnEnglish = words(
        """
        сәлем қалай рақмет жақсы бұл және жоқ бар кітап мектеп бүгін ертең қазір менің сенің
        біздің сіздің жұмыс қазақша ағылшынша мүмкін керек әңгіме денсаулық мұғалім
        """)

    // MARK: - Sentences

    static let russianSentences = [
        "я еще не сделал домашнее задание",
        "он ещё не пришёл, но скоро будет",
        "тот самый человек, который звонил вчера",
        "тут и там было много людей",
        "вот так мы и живем",
        "как дела? что нового?",
        "мне нужно сделать это до завтра",
        "это не так просто, как кажется",
        "где ты был вчера вечером?",
        "кто это сказал?",
        "уже поздно, пора спать",
        "все хорошо, спасибо",
        "давай встретимся завтра утром",
        "я тебе потом позвоню",
        "она сказала, что придет позже",
        "мы были там в прошлом году",
        "у нас все в порядке",
        "почему ты так думаешь?",
        "надо купить хлеб и молоко",
        "сколько это стоит?",
        "можно я возьму твою ручку?",
        "спасибо за помощь, очень выручил",
        "пожалуйста, отправь мне файл",
        "я не знаю, что делать",
        "он работает в большой компании",
        "мы с тобой еще поговорим",
        "тот, кто рано встает",
        "тут есть над чем подумать",
        "там видно будет",
        "так и быть, я согласен",
        "еще раз спасибо",
        "ну и что теперь",
        "да, я тоже так думаю",
        "нет, это не мой телефон",
        "вот это да",
        "как же так получилось",
        "что ты имеешь в виду?",
        "это было очень интересно",
        "уже иду",
        "мне тоже",
        "всем привет",
        "его нет дома",
        "она дома",
        "они уехали в отпуск",
        "кто там?",
        "где мои ключи?",
        "чем могу помочь?",
        "тем более что времени мало",
        "в том числе и я",
        "на той неделе",
        "с ней все хорошо",
        "о нем ничего не слышно",
        "у них все получилось",
        "для вас и для нас",
        "вам помочь?",
        "нам пора",
        "сам не знаю",
        "раз, два, три",
        "дом, милый дом",
        "мир тесен",
        "год назад",
        "через час",
        "бог с ним",
        "было бы неплохо",
        "тебе понравится",
        "люблю тебя",
        "хорошо, договорились",
        "жизнь прекрасна",
        "объект не найден",
        "съел и доволен",
        "эти люди мне помогли",
        "этот вопрос уже решен",
    ]

    static let englishSentences = [
        "the quick brown fox jumps over the lazy dog",
        "i will send you the file tomorrow",
        "can you check the latest version please",
        "thanks for your help, it works now",
        "what time is the meeting today?",
        "let me know if you need anything else",
        "we should fix this bug before the release",
        "i don't think that's a good idea",
        "it's not working on my computer",
        "where did you put the keys?",
        "she said she would be late",
        "they're going to the store",
        "how are you doing?",
        "see you later",
        "top of the list",
        "her phone is off",
        "his car is new",
        "not now, maybe later",
        "but why not?",
        "for the record, i was right",
        "you and me",
        "this is the end",
        "all good, thanks",
        "one more time",
        "yes, of course",
        "ok, sounds good",
        "hey, what's up?",
        "please update the app",
        "the build failed again",
        "open the link in a new tab",
        "i love this song",
        "call me when you get home",
        "happy birthday to you",
        "good morning everyone",
        "the project is almost done",
        "read the docs first",
        "check your email",
        "the password is wrong",
        "click the button to start",
        "who is there?",
    ]

    static let kazakhSentences = [
        "сәлеметсіз бе, қалайсыз?",
        "мен жақсымын, рақмет",
        "бұл менің кітабым",
        "біз ертең келеміз",
        "сіз қайда тұрасыз?",
        "олар мектепте оқиды",
        "бүгін ауа райы жақсы",
        "маған көмектесіңізші",
        "қазақ тілі өте әдемі",
        "ол үйде жоқ",
        "бар және жоқ",
        "менің атым Айдар",
        "сен не істеп жатырсың?",
        "қазір уақыт жоқ",
        "кешіріңіз, түсінбедім",
        "бұл жерде кім бар?",
        "жұмыс көп",
        "бәрі жақсы болады",
        "рахмет, сау болыңыз",
        "Алматы үлкен қала",
    ]

    // MARK: - Mixed and context cases

    /// One segment of a mixed sentence: the text, the layout it was meant for, and the layout
    /// that was actually active while typing it.
    struct Segment {
        var text: String
        var intended: Lang
        var active: Lang
    }

    /// A sentence and what the screen must say once the user is done typing it.
    struct Case {
        var name: String
        var segments: [Segment]
        var expected: String
    }

    private static func seg(_ text: String, _ intended: Lang, on active: Lang? = nil) -> Segment {
        Segment(text: text, intended: intended, active: active ?? intended)
    }

    /// Correctly typed mixed text: every word on the layout of its own script, the way a person
    /// switches layout to type a brand name. Separators travel with the word before them.
    static func natural(_ text: String, cyrillic: Lang) -> [Segment] {
        var segments: [Segment] = []
        var current = ""
        var currentLang: Lang?
        for character in text {
            var lang = currentLang
            if character.isLetter {
                let isLatin = character.unicodeScalars.allSatisfy { $0.value < 0x0250 }
                lang = isLatin ? .en : cyrillic
            }
            if let lang, lang != currentLang, !current.isEmpty, let currentLang {
                segments.append(Segment(text: current, intended: currentLang, active: currentLang))
                current = ""
            }
            currentLang = lang ?? currentLang ?? cyrillic
            current.append(character)
        }
        if !current.isEmpty, let currentLang {
            segments.append(Segment(text: current, intended: currentLang, active: currentLang))
        }
        return segments
    }

    private static func mixed(_ name: String, _ text: String, _ cyrillic: Lang = .ru) -> Case {
        Case(name: name, segments: natural(text, cyrillic: cyrillic), expected: text)
    }

    /// Real text mixes scripts on purpose. Brands and terms inside Russian or Kazakh sentences
    /// are deliberate and must survive; garble inside a Russian sentence must be fixed.
    static let mixedCases: [Case] = [
        mixed("brands inside Russian", "скачай Telegram и WhatsApp "),
        mixed("GitHub inside Russian", "открой GitHub, там все есть "),
        mixed("iPhone inside Russian", "мой iPhone опять сел "),
        mixed("Zoom inside Russian", "созвонимся в Zoom завтра "),
        mixed("Slack inside Russian", "напиши мне в Slack "),
        mixed("Google inside Russian", "просто загугли в Google "),
        mixed("API term inside Russian", "это API не работает "),
        mixed("email inside Russian", "пришли на email пожалуйста "),
        mixed("English term inside Russian", "нужно сделать deploy на сервер "),
        mixed("English word inside Russian", "это был не баг, а feature "),
        mixed("Wolt inside Russian", "закажи еду в Wolt или Glovo "),
        mixed("Kaspi inside Russian", "переведи на Kaspi пожалуйста "),
        mixed("English phrase inside Russian", "он сказал ok и ушел "),
        mixed("tot inside English", "a tot of whisky "),
        mixed("Kazakh with iPhone", "менің iPhone телефоным ", .kk),
        mixed("Kazakh with Instagram", "Instagram парақшама жаз ", .kk),
        Case(
            name: "Russian word typed on Kazakh layout",
            segments: [seg("Астана ", .ru, on: .kk)], expected: "Астана "),
        Case(
            name: "number on Kazakh layout",
            segments: [seg("мне ", .ru), seg("25", .en, on: .kk), seg(" лет ", .ru)],
            expected: "мне 25 лет "),
        Case(
            name: "garble after Russian context",
            segments: [seg("мне нужно ", .ru), seg("привет ", .ru, on: .en)],
            expected: "мне нужно привет "),
        Case(
            name: "garble after Russian context 2",
            segments: [seg("я думаю ", .ru), seg("что это ", .ru, on: .en)],
            expected: "я думаю что это "),
        Case(
            name: "short garble after Russian context",
            segments: [seg("ну ", .ru), seg("как дела ", .ru, on: .en)],
            expected: "ну как дела "),
        Case(
            name: "English sentence then Russian garble",
            segments: [seg("ok ", .en), seg("спасибо ", .ru, on: .en)],
            expected: "ok спасибо "),
        Case(
            name: "Russian garble sentence",
            segments: [seg("привет, как дела? ", .ru, on: .en)],
            expected: "привет, как дела? "),
        Case(
            name: "punctuation keys inside garble",
            segments: [seg("было хорошо, тебе тоже ", .ru, on: .en)],
            expected: "было хорошо, тебе тоже "),
        Case(
            name: "love you garble", segments: [seg("я тебя люблю ", .ru, on: .en)],
            expected: "я тебя люблю "),
        Case(
            name: "English garble on Russian",
            segments: [seg("hello world ", .en, on: .ru)], expected: "hello world "),
        Case(
            name: "Kazakh digits garble",
            segments: [seg("сәлеметсіз бе ", .kk, on: .ru)], expected: "сәлеметсіз бе "),
        Case(
            name: "Kazakh on English garble",
            segments: [seg("сәлем қалай ", .kk, on: .en)], expected: "сәлем қалай "),
        Case(
            name: "capitalised garble keeps its capital",
            segments: [seg("Привет ", .ru, on: .en)], expected: "Привет "),
        Case(
            name: "еще mid-sentence stays",
            segments: [seg("мы еще не знаем ", .ru)], expected: "мы еще не знаем "),
        Case(
            name: "тот mid-sentence stays",
            segments: [seg("это тот самый файл ", .ru)], expected: "это тот самый файл "),
        Case(
            name: "тут mid-sentence stays",
            segments: [seg("я тут подумал ", .ru)], expected: "я тут подумал "),
        Case(
            name: "English her stays",
            segments: [seg("i told her about it ", .en)], expected: "i told her about it "),
        Case(
            name: "English top stays",
            segments: [seg("at the top of the page ", .en)], expected: "at the top of the page "),
    ]

    // MARK: - Collisions: words that are words on both layouts

    /// Russian words whose English-layout reading is also an English word, or the reverse, typed
    /// correctly. Measured in the list and in context; they must stay.
    static let russianCollisions = words("рук руку иге еру нею туче тех них всё моё неё был")
    static let englishCollisions = words("ult tot yt vs ye kb pf nt nj ns dj bp yb her here next the but")

    /// Words with punctuation glued on, typed correctly: the punctuation keys that are letters on
    /// the other layout must not turn a real word into a "mistake".
    static let englishPunctuated = words(
        """
        hello, world. (ok) e.g. i.e. U.S. don't, it's. [link] "quote" well-known x-ray 3.14 v2.0
        a,b foo.bar end; yes: no! why? (see above) [1] co-op re-enter
        """)
    static let russianPunctuated = words(
        """
        привет, да. (это) кто-то из-за т.е. т.к. и.т.д. что-нибудь по-русски «цитата» ну-ка
        """)

    /// Real words the dictionaries do not have, typed on the wrong layout: only the character
    /// model can catch these, and it may only offer.
    static let russianRareOnEnglish = words(
        "приветики зашибись ништяк кринжатина лайкнуть загуглить чатик созвонимся задеплоить")

    // MARK: - Out-of-vocabulary text that must stay

    /// Names, slang and abbreviations: things no dictionary has, typed correctly.
    static let russianUnknown = words(
        """
        Айдар Нурлан Айгерим Ерлан Данияр Асель Жанна Алматы Шымкент Караганда норм окей ок спс
        пж плз лол кек чел комп инет прив пасиб щас чё чо ваще канеш оч хз имхо кринж го лан
        """)

    static let englishUnknown = words(
        """
        Aidar Almaty Aigerim Nurlan lol omg btw idk imo afk asap tbh brb thx pls gg np wtf irl
        kinda gonna wanna gotta yeah nope yep hmm ugh lmao rofl xoxo selfie vlog
        """)

    /// Typos of frequent words, typed on the right layout. A misspelling is not a layout
    /// mistake, and turning `превет` into Latin garble would be the worst kind of help.
    static func typos(of words: [String], limit: Int) -> [String] {
        var result: [String] = []
        for word in words where word.count >= 4 {
            var characters = Array(word)
            // Swap two neighbouring letters in the middle.
            let middle = characters.count / 2
            characters.swapAt(middle - 1, middle)
            let swapped = String(characters)
            if swapped != word { result.append(swapped) }
            // Drop one letter.
            var dropped = Array(word)
            dropped.remove(at: dropped.count / 2)
            result.append(String(dropped))
            if result.count >= limit { break }
        }
        return Array(result.prefix(limit))
    }
}
