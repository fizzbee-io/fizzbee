# Publishing Guide for @fizzbee/mbt

This guide explains how to publish the `@fizzbee/mbt` package to npm.

## Prerequisites

1. **npm account**: Create one at https://www.npmjs.com/signup if you don't have one
2. **Organization access**: For scoped packages like `@fizzbee/mbt`, you need:
   - Either: Create the `@fizzbee` organization on npm (https://www.npmjs.com/org/create)
   - Or: Have access granted to the existing `@fizzbee` organization

## First-Time Setup

### 1. Login to npm

```bash
npm login
```

Enter your npm username, password, and email when prompted. For two-factor authentication, you'll also need your OTP token.

### 2. Verify your login

```bash
npm whoami
```

This should display your npm username.

### 3. Check organization access (for scoped packages)

```bash
npm org ls fizzbee
```

If the organization doesn't exist, create it or remove the `@fizzbee/` scope from the package name.

### 4. Test the build

Before publishing, ensure everything builds correctly:

```bash
# Clean previous builds
npm run clean

# Install dependencies (if not already done)
npm install

# Run the build
npm run build
```

Verify that the `dist/` directory contains:
- `index.js`
- `index.d.ts`
- Other compiled files

### 5. Test the package locally (optional but recommended)

Create a test project and install your package locally:

```bash
# In a different directory
mkdir test-fizzbee-mbt
cd test-fizzbee-mbt
npm init -y

# Install your package from the local directory
npm install /Users/jp/src/fizzbee/mbt/lib/typescript

# Test that you can import it
node -e "const mbt = require('@fizzbee/mbt'); console.log(mbt);"
```

### 6. Dry run the publish

Test what will be published without actually publishing:

```bash
npm publish --dry-run
```

This shows you:
- What files will be included
- The package size
- Any warnings or errors

### 7. Publish to npm

For a scoped package, you have two options:

**Option A: Public package (recommended for open source)**
```bash
npm publish --access public
```

**Option B: Private package (requires paid npm account)**
```bash
npm publish
```

### 8. Verify the publication

After publishing:

```bash
# Check on npm website
open https://www.npmjs.com/package/@fizzbee/mbt

# Or install in a test project
npm install @fizzbee/mbt
```

## Publishing Subsequent Releases

### 1. Make your changes

Edit code, fix bugs, add features, etc.

### 2. Update the version

Use npm's version command which:
- Updates package.json
- Creates a git commit
- Creates a git tag

```bash
# For bug fixes (0.1.0 -> 0.1.1)
npm version patch

# For new features (0.1.0 -> 0.2.0)
npm version minor

# For breaking changes (0.1.0 -> 1.0.0)
npm version major
```

Or manually edit `package.json` and update the version number.

### 3. Update CHANGELOG.md (recommended)

Document what changed in this version:

```bash
# Edit CHANGELOG.md
# Add a new section for the version with changes
```

### 4. Build and test

```bash
npm run clean
npm run build

# Run your tests if you have them
npm test
```

### 5. Commit changes (if not using npm version)

```bash
git add .
git commit -m "Release v0.x.x"
git tag v0.x.x
```

### 6. Publish

```bash
npm publish --access public
```

### 7. Push to git

```bash
git push
git push --tags
```

## Version Numbering (Semantic Versioning)

Follow semantic versioning (semver):

- **MAJOR** version (1.0.0 -> 2.0.0): Breaking changes
- **MINOR** version (1.0.0 -> 1.1.0): New features, backward compatible
- **PATCH** version (1.0.0 -> 1.0.1): Bug fixes, backward compatible

## Pre-release Versions

For beta or alpha releases:

```bash
# Create a beta version (0.1.0 -> 0.1.1-beta.0)
npm version prerelease --preid=beta

# Publish with a tag
npm publish --tag beta --access public
```

Users can install with:
```bash
npm install @fizzbee/mbt@beta
```

## Troubleshooting

### "You do not have permission to publish"

- Verify you're logged in: `npm whoami`
- Check organization access: `npm org ls fizzbee`
- Ensure you have permissions in the organization

### "Package name already exists"

- The package name is taken
- Check if you own it: https://www.npmjs.com/package/@fizzbee/mbt
- Consider using a different name

### "Version already published"

- You can't republish the same version
- Increment the version: `npm version patch`

### Build fails before publish

- Check that `prepublishOnly` script works: `npm run prepublishOnly`
- Ensure all dependencies are installed: `npm install`
- Check for TypeScript errors: `npm run build`

## Best Practices

1. **Always test before publishing**: Run `npm publish --dry-run`
2. **Use semver correctly**: Follow semantic versioning
3. **Keep CHANGELOG.md updated**: Document changes for users
4. **Tag releases in git**: Use `git tag v0.x.x`
5. **Don't publish secrets**: Check `.npmignore` and `files` in package.json
6. **Test the published package**: Install it in a separate project
7. **Use --access public for open source**: Scoped packages are private by default
8. **Consider npm provenance**: Use `npm publish --provenance` for supply chain security (requires GitHub Actions)

## Automated Publishing (Optional)

Consider setting up GitHub Actions to publish automatically:

```yaml
# .github/workflows/publish.yml
name: Publish to npm

on:
  release:
    types: [created]

jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-node@v3
        with:
          node-version: '18'
          registry-url: 'https://registry.npmjs.org'
      - run: npm ci
      - run: npm run build
      - run: npm publish --access public --provenance
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
```

## Quick Reference

```bash
# First time
npm login
npm run build
npm publish --dry-run
npm publish --access public

# Subsequent releases
npm version patch  # or minor, or major
npm run build
npm publish --access public
git push --tags
```
