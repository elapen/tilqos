# Signing and notarization settings shared by build.sh and make-dmg.sh.
# Source this file; do not run it.
#
# Environment variables (all optional):
#
#   SIGN_IDENTITY    Name of the codesign identity to use; "-" forces an ad-hoc signature.
#                    Default: picked from the keychain, preferring "Developer ID
#                    Application" over "Apple Development". If the keychain has neither,
#                    the build falls back to AD-HOC signing (see the note printed below).
#
#   NOTARY_PROFILE   Name of a keychain profile made once with
#                      xcrun notarytool store-credentials "<profile>" \
#                          --apple-id "$APPLE_ID" --team-id "$TEAM_ID"
#                    (notarytool asks for the app-specific password and stores it in the
#                    keychain, so no secret ever sits in the environment or on disk).
#   APPLE_ID, TEAM_ID
#                    Alternative to NOTARY_PROFILE: notarytool prompts for the app-specific
#                    password on every submission.
#
# Notarization needs a "Developer ID Application" identity and one of the two credential
# forms above. No identity, profile, Apple ID or team ID is hard-coded anywhere in this
# repository.

# Prints the name of the best codesigning identity in the keychain, or nothing.
detect_identity() {
  local found kind
  for kind in "Developer ID Application" "Apple Development"; do
    found=$(security find-identity -v -p codesigning 2>/dev/null \
      | grep "$kind" | head -1 | sed -E 's/^[^"]*"([^"]*)".*$/\1/' || true)
    if [ -n "$found" ]; then printf '%s' "$found"; return; fi
  done
}

# Sets SIGN_IDENTITY (never empty) and ADHOC (1 when the signature is ad-hoc).
resolve_signing_identity() {
  SIGN_IDENTITY="${SIGN_IDENTITY:-$(detect_identity)}"
  if [ -z "$SIGN_IDENTITY" ]; then SIGN_IDENTITY="-"; fi
  if [ "$SIGN_IDENTITY" = "-" ]; then ADHOC=1; else ADHOC=0; fi
}

# The note shown whenever the signature is ad-hoc.
print_adhoc_note() {
  {
    printf '\033[1;33mnote:\033[0m no code-signing identity in the keychain: signing AD-HOC.\n'
    printf '      macOS ties the Accessibility / Input Monitoring grants to the code identity,\n'
    printf '      and an ad-hoc signature is different for every build, so after each rebuild\n'
    printf '      you must remove the old "Tilqos" entry in System Settings > Privacy & Security\n'
    printf '      (Accessibility, Input Monitoring) and approve the new build again. A stable\n'
    printf '      "Apple Development" identity (Xcode > Settings > Accounts > Manage Certificates;\n'
    printf '      a free Apple ID is enough) avoids that. Ad-hoc builds cannot be notarized.\n'
  } >&2
}

# Fills NOTARY_ARGS with the notarytool credential arguments taken from the environment
# (empty when none are set), so `[ ${#NOTARY_ARGS[@]} -gt 0 ]` means "can notarize".
resolve_notary_args() {
  NOTARY_ARGS=()
  if [ -n "${NOTARY_PROFILE:-}" ]; then
    NOTARY_ARGS=(--keychain-profile "$NOTARY_PROFILE")
  elif [ -n "${APPLE_ID:-}" ] && [ -n "${TEAM_ID:-}" ]; then
    NOTARY_ARGS=(--apple-id "$APPLE_ID" --team-id "$TEAM_ID")
  fi
}

# True when SIGN_IDENTITY can be notarized.
is_developer_id() {
  case "$SIGN_IDENTITY" in
    "Developer ID Application"*) return 0 ;;
    *) return 1 ;;
  esac
}
