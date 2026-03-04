#!/usr/bin/env bash
# Install FizzBee Claude Code skills and reference docs to ~/.claude/skills/
# Makes FizzBee spec writing, checking, debugging, and MBT skills available
# in Claude Code across all your projects.
#
# Usage:
#   bash install-claude-skills.sh           # install/update all skills + docs
#   bash install-claude-skills.sh --check   # show what would be installed
#   bash install-claude-skills.sh --remove  # remove installed skills + docs

set -euo pipefail

REPO="fizzbee-io/fizzbee"
BRANCH="main"
RAW="https://raw.githubusercontent.com/${REPO}/${BRANCH}"
SKILLS_DIR="${HOME}/.claude/skills"
DOCS_DIR="${SKILLS_DIR}/fizzbee-docs"

SKILLS=(fizz-spec fizz-check fizz-debug fizz-mbt)

# Reference docs downloaded alongside skills so they're available in any project
DOCS=(
  "examples/references/LANGUAGE_REFERENCE.md"
  "examples/references/GOTCHAS.md"
  "examples/references/PERFORMANCE_GUIDE.md"
  "examples/references/VERIFICATION_GUIDE.md"
  "examples/references/README.md"
)

# Curated example specs covering the main feature areas (parallel arrays)
EXAMPLE_NAMES=(
  "01-counter.fizz"
  "09-assertions.fizz"
  "11-roles.fizz"
  "13-two-phase-commit.fizz"
  "14-fault-injection.fizz"
  "16-symmetry.fizz"
)
EXAMPLE_PATHS=(
  "examples/references/01-02-atomic-action/Counter.fizz"
  "examples/references/09-01-always-assertion/Counter.fizz"
  "examples/references/11-01-simple-role/Account.fizz"
  "examples/references/13-02-01-two-phase-commit/TwoPhaseCommit.fizz"
  "examples/references/14-01-crash-on-yield/CrashOnYield.fizz"
  "examples/references/16-05-nominal-symmetry/NominalSymmetry.fizz"
)

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

check_deps() {
  if command -v curl &>/dev/null; then
    DOWNLOAD="curl -fsSL"
  elif command -v wget &>/dev/null; then
    DOWNLOAD="wget -qO-"
  else
    echo -e "${RED}Error: curl or wget is required${NC}" >&2
    exit 1
  fi
}

do_check() {
  echo "Would install to: ${SKILLS_DIR}/"
  echo ""
  echo "Skills:"
  for skill in "${SKILLS[@]}"; do
    if [[ -f "${SKILLS_DIR}/${skill}/SKILL.md" ]]; then
      echo -e "  ${YELLOW}↻${NC} ${skill}  (already installed, would update)"
    else
      echo -e "  ${GREEN}+${NC} ${skill}  (new)"
    fi
  done
  echo ""
  echo "Reference docs (${DOCS_DIR}/):"
  for doc in "${DOCS[@]}"; do
    name=$(basename "$doc")
    if [[ -f "${DOCS_DIR}/${name}" ]]; then
      echo -e "  ${YELLOW}↻${NC} ${name}  (already installed, would update)"
    else
      echo -e "  ${GREEN}+${NC} ${name}  (new)"
    fi
  done
  echo ""
  echo "Example specs (${DOCS_DIR}/examples/):"
  for name in "${EXAMPLE_NAMES[@]}"; do
    if [[ -f "${DOCS_DIR}/examples/${name}" ]]; then
      echo -e "  ${YELLOW}↻${NC} ${name}  (already installed, would update)"
    else
      echo -e "  ${GREEN}+${NC} ${name}  (new)"
    fi
  done
}

do_install() {
  mkdir -p "${SKILLS_DIR}" "${DOCS_DIR}" "${DOCS_DIR}/examples"

  echo "Installing FizzBee Claude skills to ${SKILLS_DIR}/"
  echo ""
  for skill in "${SKILLS[@]}"; do
    mkdir -p "${SKILLS_DIR}/${skill}"
    $DOWNLOAD "${RAW}/.claude/skills/${skill}/SKILL.md" > "${SKILLS_DIR}/${skill}/SKILL.md"
    echo -e "  ${GREEN}✓${NC} ${skill}"
  done

  echo ""
  echo "Installing reference docs to ${DOCS_DIR}/"
  echo ""
  for doc in "${DOCS[@]}"; do
    name=$(basename "$doc")
    $DOWNLOAD "${RAW}/${doc}" > "${DOCS_DIR}/${name}"
    echo -e "  ${GREEN}✓${NC} ${name}"
  done

  echo ""
  echo "Installing example specs to ${DOCS_DIR}/examples/"
  echo ""
  for i in "${!EXAMPLE_NAMES[@]}"; do
    $DOWNLOAD "${RAW}/${EXAMPLE_PATHS[$i]}" > "${DOCS_DIR}/examples/${EXAMPLE_NAMES[$i]}"
    echo -e "  ${GREEN}✓${NC} ${EXAMPLE_NAMES[$i]}"
  done

  echo ""
  echo -e "${GREEN}Done.${NC} Skills and docs are now available in all Claude Code sessions."
  echo ""
  echo "To use:"
  echo "  - Claude Code will auto-invoke the right skill based on context"
  echo "  - Or invoke manually: /fizz-spec  /fizz-check  /fizz-debug  /fizz-mbt"
  echo ""
  echo "To update after upgrading fizzbee, run:  fizz install-skills"
}

do_remove() {
  echo "Removing FizzBee Claude skills and docs..."
  for skill in "${SKILLS[@]}"; do
    if [[ -d "${SKILLS_DIR}/${skill}" ]]; then
      rm -rf "${SKILLS_DIR:?}/${skill}"
      echo -e "  ${RED}✗${NC} ${skill}"
    else
      echo "  - ${skill}  (not installed)"
    fi
  done
  if [[ -d "${DOCS_DIR}" ]]; then
    rm -rf "${DOCS_DIR:?}"
    echo -e "  ${RED}✗${NC} fizzbee-docs/"
  else
    echo "  - fizzbee-docs/  (not installed)"
  fi
  echo ""
  echo "Done."
}


check_deps

case "${1:-}" in
  --check)   do_check ;;
  --remove)  do_remove ;;
  *)         do_install ;;
esac
