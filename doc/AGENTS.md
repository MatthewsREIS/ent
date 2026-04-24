<!-- Generated: 2026-02-08 -->
<!-- Parent: ../AGENTS.md -->

# AGENTS.md — doc

This document provides guidance for AI agents working in the **doc** directory.

**Directory**: `entgo.io/ent/doc`
**Purpose**: Documentation sources (markdown) and website (Docusaurus)
**Content**: User guides, API docs, migration guides, FAQs, blog posts, and interactive website

---

## Overview

The `doc/` directory contains all user-facing documentation for the ent framework:

- **`md/`**: Source markdown files for core documentation
- **`website/`**: Docusaurus-based website (source and build)

Documentation is published to https://entgo.io/ and serves as:

- Getting started guides
- API reference
- Feature walkthroughs
- Migration guides
- Troubleshooting (FAQ)
- Blog posts and announcements

---

## Directory Structure

```
doc/
├── md/                          # Source markdown documentation
│   ├── components/              # Reusable MDX components
│   ├── migration/               # Migration guides (versioned)
│   ├── versioned/               # Previous version docs
│   ├── getting-started.mdx      # Entry point tutorial
│   ├── crud.mdx                 # CRUD operations guide
│   ├── code-gen.md              # Code generation guide
│   ├── features.md              # Feature overview
│   ├── faq.md                   # Frequently asked questions
│   ├── graphql.md               # GraphQL integration
│   ├── privacy.md               # Privacy/auth guide
│   └── ... (20+ more)
│
└── website/                     # Docusaurus website
    ├── blog/                    # Blog posts (*.mdx)
    ├── src/                     # React components and pages
    ├── static/                  # Assets (images, etc.)
    ├── docusaurus.config.js     # Site configuration
    ├── sidebars.js              # Navigation structure
    └── package.json             # Node dependencies
```

---

## Documentation Files

### Core Guides

**High-Traffic, Critical Files**:

| File | Purpose | Update Frequency |
|------|---------|------------------|
| `md/getting-started.mdx` | Tutorial for new users | Medium |
| `md/crud.mdx` | Create/Read/Update/Delete patterns | Medium |
| `md/features.md` | Feature overview and capabilities | Low |
| `md/faq.md` | Frequently asked questions | Medium |
| `md/code-gen.md` | Code generation configuration | Low |

### Feature Documentation

| File | Purpose |
|------|---------|
| `md/schema-def.md` | Schema definition (fields, edges, indexes) |
| `md/queries.md` | Query builders and filtering |
| `md/mutations.md` | Mutation operations |
| `md/relationships.md` | Edge and relationship patterns |
| `md/hooks.md` | Mutation hooks and interceptors |
| `md/privacy.md` | Privacy policies and authorization |
| `md/graphql.md` | GraphQL integration |
| `md/migration.md` | Schema migrations |
| `md/dialects.md` | Database dialect support |

### Migration & Troubleshooting

| File | Purpose |
|------|---------|
| `md/migration/` | Versioned migration guides (v0→v1, v1→v2, etc.) |
| `md/faq.md` | Troubleshooting and common questions |
| `md/ci.mdx` | CI/CD integration guide |

### Metadata Files

| File | Purpose |
|------|---------|
| `md/components/` | Reusable MDX components (admonitions, code blocks, etc.) |
| `website/sidebars.js` | Navigation menu definition (controls doc order) |
| `website/docusaurus.config.js` | Site metadata, title, social links |

---

## Common Tasks

### Writing or Updating Documentation

1. **Identify the target file**: Does it exist in `md/`? Or add new?
2. **Edit the markdown**: Update or create `.md` or `.mdx` file
3. **Check syntax**: Valid Markdown/MDX with proper frontmatter
4. **Preview locally** (if Docusaurus is set up):
   ```bash
   cd doc/website
   npm install  # First time only
   npm start    # Runs dev server at http://localhost:3000
   ```
5. **Verify links**: Internal links use `[text](path)` format (relative to `md/`)
6. **Test code examples**: Ensure commands and snippets are accurate

### Adding a New Documentation Page

1. **Create the file**: `doc/md/mynewpage.md` or `.mdx`
2. **Add frontmatter** (YAML header):
   ```yaml
   ---
   title: "My New Page Title"
   ---
   ```
3. **Write content**: Standard Markdown with optional MDX components
4. **Update sidebar**: Edit `website/sidebars.js` to add navigation entry
5. **Test**: `npm start` in `website/` and verify page appears

Example frontmatter:

```yaml
---
title: "Advanced Patterns"
sidebar_label: "Patterns"
description: "Learn advanced ent patterns and techniques"
---
```

### Writing Blog Posts

1. **Create file**: `doc/website/blog/YYYY-MM-DD-slug.mdx`
2. **Add frontmatter**:
   ```yaml
   ---
   title: "Post Title"
   authors: [name]
   tags: [tag1, tag2]
   ---
   ```
3. **Write content**: Markdown/MDX with code examples
4. **Test**: Appears automatically on blog index

Example:

```yaml
---
title: "Ent v0.11.0 Released"
authors: [rotemtam]
tags: [release, announcement]
---

## New Features

- Feature 1
- Feature 2
```

### Updating Navigation/Sidebar

Edit `website/sidebars.js` to control documentation order and structure:

```javascript
module.exports = {
  docs: [
    {
      type: 'category',
      label: 'Getting Started',
      items: [
        'getting-started',
        'installation',
      ],
    },
    // More sections...
  ],
};
```

---

## Markdown/MDX Guidelines

### Frontmatter (Required for `.md` files)

```yaml
---
title: "Page Title"
sidebar_label: "Short Label"  # Optional
description: "Brief description"
---
```

### Heading Hierarchy

```markdown
# Page Title (H1 - only one per page, usually in frontmatter)

## Section (H2 - use for main sections)

### Subsection (H3 - for details within section)
```

### Code Blocks

Use triple backticks with language:

````markdown
```go
package main

func main() {
	fmt.Println("Hello, ent!")
}
```
````

### Internal Links

Use relative paths:

```markdown
[Link to CRUD guide](./crud)
[Link to section](#subsection-name)
```

### Admonitions (Note, Tip, Warning)

Use MDX components from `components/`:

```markdown
:::note
This is a note box
:::

:::tip
This is a tip
:::

:::warning
This is a warning
:::
```

### Code Tabs (Multiple Language Examples)

Use Docusaurus Tabs component:

```markdown
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

<Tabs>
<TabItem value="go" label="Go">

```go
// Go example
```

</TabItem>
<TabItem value="sql" label="SQL">

```sql
-- SQL example
```

</TabItem>
</Tabs>
```

---

## Website Structure

### Configuration (website/docusaurus.config.js)

Controls site metadata:

```javascript
module.exports = {
  title: 'ent',
  url: 'https://entgo.io',
  baseUrl: '/',
  navbar: { /* ... */ },
  footer: { /* ... */ },
};
```

### Sidebar (website/sidebars.js)

Defines documentation navigation. Example:

```javascript
module.exports = {
  docs: [
    {
      type: 'category',
      label: 'Guides',
      items: ['getting-started', 'crud', 'schema-def'],
    },
  ],
};
```

### Blog Posts (website/blog/)

Located in `blog/`, automatically indexed. Format: `YYYY-MM-DD-slug.mdx`

### Static Assets (website/static/)

Images, CSS, JavaScript. Access via `/img/`, `/css/`, etc.

---

## Testing & Verification

### Local Preview

```bash
cd doc/website
npm install      # First time
npm start        # Runs at localhost:3000
npm run build    # Builds static site
```

### Build Verification

Ensure documentation builds without errors:

```bash
cd doc/website
npm run build
# Check for warnings/errors in output
```

### Link Checking

Internal and external links should be valid:

- **Internal**: `[text](./relative/path)` format
- **External**: Full URLs `https://example.com`

### Code Example Verification

For critical tutorials, verify code examples work:

1. Copy code snippets into a test project
2. Run/compile to confirm they work
3. Note any dependencies or setup required

---

## Common Pitfalls

### Pitfall 1: Broken Internal Links

**Wrong**: `[text](full_url_with_domain)`
**Right**: `[text](./relative/path)` or use Docusaurus link format

### Pitfall 2: Outdated Code Examples

**Wrong**: Code examples that don't compile or don't match current API
**Right**: Verify examples against actual codebase; add notes if API changes

### Pitfall 3: Missing Frontmatter

**Wrong**: `.md` file without YAML header
**Right**: Always include `---\ntitle: "..."\n---` at the top

### Pitfall 4: Incorrect Sidebar Configuration

**Wrong**: References to files that don't exist in sidebar
**Right**: Ensure every `sidebar_label` points to actual file in `md/`

### Pitfall 5: Inconsistent Terminology

**Wrong**: Switching between "entity," "node," "model" without explanation
**Right**: Define key terms; use consistently throughout

### Pitfall 6: No Table of Contents for Long Pages

**Wrong**: Dense text without section headers
**Right**: Use H2/H3 headers to break up content; Docusaurus auto-generates TOC

---

## Verification Checklist

Before marking documentation complete:

- [ ] File is in correct location (`doc/md/` or `doc/website/blog/`)
- [ ] Frontmatter is valid YAML with required fields
- [ ] Markdown syntax is correct (no rendering errors)
- [ ] Internal links use relative paths
- [ ] External links are full URLs starting with `http`
- [ ] Code examples are accurate and tested
- [ ] Heading hierarchy is logical (no jumps like H1→H3)
- [ ] Admonitions use correct syntax (note/tip/warning)
- [ ] Page title matches navigation label (if applicable)
- [ ] Sidebar updated if adding new page
- [ ] Local preview works: `npm start`
- [ ] Build completes without errors: `npm run build`

---

## Useful Commands

```bash
# Preview documentation locally
cd doc/website && npm start

# Build static site
cd doc/website && npm run build

# Install dependencies (first time)
cd doc/website && npm install

# Check for build errors
cd doc/website && npm run build 2>&1 | grep -i "error"
```

---

## Key Files Reference

| File | Purpose | When to Touch |
|------|---------|---------------|
| `md/*.md` | Documentation source | Adding/updating guides |
| `md/migration/*.md` | Version-specific guides | Writing migration guides for new versions |
| `website/sidebars.js` | Navigation structure | Adding new pages or reordering sections |
| `website/docusaurus.config.js` | Site config | Changing title, URL, social links, etc. |
| `website/blog/*.mdx` | Blog posts | Publishing announcements or detailed articles |
| `website/static/img/` | Images/graphics | Adding diagrams or screenshots |

---

## Questions? Check These Resources

1. **Docusaurus guide**: https://docusaurus.io/docs/markdown-features
2. **MDX components**: See examples in `md/components/`
3. **Existing pages**: Reference similar pages (e.g., `md/crud.mdx`)
4. **Blog examples**: See `website/blog/` for post formatting
5. **Configuration**: Check `website/docusaurus.config.js` and `website/sidebars.js`

---

**Last Updated**: 2026-02-08
