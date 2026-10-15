import XCTest

@testable import TilqosKit

/// What the Settings window's buttons and switches do to stored data: the "clear all" and
/// "reset" buttons delete plain-text lists and learned hashes, and the log switch reaches
/// the logger.
@MainActor
final class SettingsPrivacyTests: XCTestCase {
    private var scratch: ScratchDefaults!
    private var model: SettingsModel!

    override func setUp() {
        scratch = ScratchDefaults.make()
        model = SettingsModel(
            prefs: Prefs(defaults: scratch.defaults), inputSources: InputSourceManager(),
            pipeline: nil)
        model.learner = Learner(defaults: scratch.defaults)
    }

    override func tearDown() {
        Log.diagnosticsEnabled = false
        scratch.remove()
    }

    func testClearAllExceptionWordsEmptiesTheList() {
        model.addException("Qwerty")
        model.addException("zoom")
        XCTAssertEqual(model.exceptions.count, 2)
        model.clearExceptionWords()
        XCTAssertTrue(model.exceptions.isEmpty)
        XCTAssertNil(
            scratch.defaults.object(forKey: "wordExceptions"),
            "the plain-text list is removed from the preferences, not just emptied")
    }

    func testClearKeepInLatinWordsEmptiesTheList() {
        model.addNeverConvert("instagram")
        model.clearNeverConvertWords()
        XCTAssertTrue(model.neverConvert.isEmpty)
    }

    func testResetLearnedWordsForgetsEverythingAndChangesTheSalt() {
        let learner = model.learner
        for word in ["tot", "ult"] { learner.rejected(typed: word) }
        XCTAssertGreaterThan(learner.counts.total, 0)
        let saltBefore = scratch.defaults.data(forKey: "learningSalt")
        model.resetLearnedVocabulary()
        XCTAssertEqual(learner.counts.total, 0)
        XCTAssertNotEqual(scratch.defaults.data(forKey: "learningSalt"), saltBefore)
    }

    func testDetailedLogSwitchReachesTheLogger() {
        XCTAssertFalse(model.prefs.diagnosticsEnabled)
        model.binding(\.diagnosticsEnabled) { $0.diagnosticsEnabled = $1 }.wrappedValue = true
        XCTAssertTrue(model.prefs.diagnosticsEnabled)
        XCTAssertTrue(Log.diagnosticsEnabled)
        model.binding(\.diagnosticsEnabled) { $0.diagnosticsEnabled = $1 }.wrappedValue = false
        XCTAssertFalse(Log.diagnosticsEnabled)
    }
}
