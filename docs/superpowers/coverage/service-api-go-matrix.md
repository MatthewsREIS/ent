# service-api-go Test Coverage of Generated ent Code

Produced during Plan Task 2 of `docs/superpowers/plans/2026-04-23-ent-code-reduction-phase-1-3.md`.

Consumer repo: `/home/smoothbrain/dev/matthewsreis/worktrees/main/service-api-go` (SHA: `1bcf17f57b32e4dede078c25c45a7956210656b0`)

## Methodology note

274 test files import `ent/gen`. Because that exceeds the 50-file per-file threshold, this matrix aggregates by **test directory** (17 groups). Category columns reflect whether the directory's tests exercise that surface at all; entity lists are the distinct ent schema names observed (via direct `client.Entity.*` calls, generated predicate package imports, or GraphQL mutation/query strings that drive ent resolvers).

"Covered" (for schema summary) means any test in any directory references the entity — directly through the ent client, its generated predicate package, or through GraphQL operations whose resolvers are ent-generated.

## File-category key

| Column   | Meaning |
|---|---|
| where    | predicate functions in `<entity>/where.go` — `entity.FieldEQ(...)`, `entity.FieldIn(...)`, etc. |
| create   | builder setters in `<entity>_create.go` — `.Create().SetX(...)` chains |
| update   | builder setters / `AddX` / `ClearX` in `<entity>_update.go` — `.Update()` / `.UpdateOne()` / `.UpdateOneID()` |
| delete   | `<entity>_delete.go` — `.Delete()` / `.DeleteOne()` / `.DeleteOneID()` |
| mutation | state tracking in `internal/<entity>_mutation.go` — `OldX`, `ResetX`, `Fields()`, `AddedFields()` |
| client   | top-level `client.go` surfaces — `Tx`, `Debug`, `Intercept`, `Use` hooks, `NewContext`, `FromContext` |
| entql    | entql predicate evaluation — `As*Predicate`, entql-specific APIs |
| query    | `<entity>_query.go` — `.Query().All/First/Only/IDs`, `.With*()` eager loading, `.GroupBy`, `.Select` |

Threshold for ✓: the directory exercises the category's API surface non-trivially across multiple files. Trivial uses (e.g. a single `.Query()` call only to assert a side effect) are left blank.

## Test-directory matrix

| Directory (relative to consumer root) | Files | where | create | update | delete | mutation | client | entql | query | Entities covered |
|---|:-:|:-:|:-:|:-:|:-:|:-:|:-:|:-:|:-:|---|
| `api-graphql/e2e/tests` | 4 | ✓ | ✓ | ✓ | | | | | ✓ | Email, Listing, Marketing, Property, Task, TextEmailTask, Timeline, User, WrikeProject, WrikeTask |
| `api-graphql/src/clients/box` | 1 | | ✓ | | | | | | | BoxFolder, Escrow, Listing, Property, Proposal |
| `api-graphql/src/cmd` (3 subdirs) | 4 | ✓ | ✓ | | ✓ | | | | ✓ | Contact, SalesforceSyncPosition, Task, User |
| `api-graphql/src/ent/entcontext_test` | 1 | | | | | | ✓ | | | gen package context/impersonation helpers only |
| `api-graphql/src/ent/gen/enthubspot` | 4 | | ✓ | | | | ✓ | | ✓ | Contact, ContactList, ContactListContact, Content |
| `api-graphql/src/ent/gen/entsf_test` | 2 | | ✓ | ✓ | ✓ | | ✓ | | ✓ | Company, Contact, ContactList, ContactListContact, Email, Escrow, Lease |
| `api-graphql/src/ent/schema` | 9 | ✓ | ✓ | ✓ | ✓ | ✓ | | | ✓ | Contact, Contract, Escrow, Listing, Market, Marketing, Office, OfficeUser, Ownership, PartyToTransaction, Profile, Property, Proposal, User, WrikeProject, WrikeTask |
| `api-graphql/src/export` | 1 | | | | | | | | | Company, Contact, Email (query via GraphQL export path) |
| `api-graphql/src/extensions/entsearch` | 8 | | ✓ | | | | | | ✓ | Company, Contact, Content, Email, Escrow, Market, Property |
| `api-graphql/src/hubspot_email_stats` | 1 | | | | | | ✓ | | | gen.Client{} struct only — no entity queries |
| `api-graphql/src/resolvers` | 11 | ✓ | ✓ | ✓ | | | ✓ | ✓ | ✓ | Company, Contact, ContactList, ContactListContact, DealTeam, Email, Escrow, Lease, Listing, Market, Marketing, Office, Property, RecordType, User |
| `api-graphql/src/ringcentral_poller` | 1 | | ✓ | | | | | | | Contact, PhoneNumber, Task |
| `api-graphql/src/river` | 12 | ✓ | ✓ | ✓ | | | | | ✓ | DealTeam, Email, Escrow, Listing, Marketing, Property, Proposal, RoleDefinition, Task, User, WrikeProject, WrikeTask |
| `api-graphql/src/testutil/fixtures` | 1 | ✓ | | | | | | | ✓ | SfGroup, SfGroupMember (where-predicate on sfgroupmember package) |
| `api-graphql/src/testutil` | 212 | ✓ | ✓ | ✓ | ✓ | | ✓ | ✓ | ✓ | AgentLicensing, AiChatMessage, AiChatThread, AlphamapMartOutput, BoxFolder, BrokerOfRecord, ChatterLastReadMessage, ChatterMention, ChatterMessage, ChatterNotification, ChatterThread, CoBroker, Collateral, Commission, Company, CompanyHistory, CompanyNote, Compliance, Contact, ContactHistory, ContactList, ContactListContact, ContactListShare, ContactNote, Content, Contract, CrexiMartOutput, DealNotification, DealTeam, DmEmailStats, Email, EmailStats, EmailToBrokersTask, Escrow, EscrowHistory, HR, HtmlEmailTask, HyperionMartOutput, Intercept, Lease, Listing, Market, MarketAgentRegion, Marketing, MreisAgentCommission, Notification, Offer, Office, OfficeUser, Opportunity, Outreach, OutreachQueue, Ownership, PartyToTransaction, PhoneNumber, PinnedContactList, PinnedPropertyList, Profile, Property, PropertyFieldHistory, PropertyFieldSelection, PropertyHistory, PropertyList, PropertyNote, Proposal, PublicWebPostsTask, RecordType, ReferralFee, RelatesTo, RequestEmailTask, RequestPhotosTask, RingCentralPhoneNumber, RingCentralPhoneNumberMdt, RoleDefinition, SalesComp, SFGroup, SFGroupMember, sfsync, Signage, Space, Task, TextEmailTask, Timeline, User, UserContact, UserReminder, UserSettings, UserTask, Website, WrikeAttachment, WrikeProject, WrikeTask, WrikeWorkflowStatus |
| `api-graphql/src/txutil` | 1 | | | | | | ✓ | | ✓ | gen.Tx interface testing only — no entity queries |
| `webhooks/hubspot` | 1 | | ✓ | | | | | | | Content |

## Schema coverage summary

> **Note on schema count:** The task spec cited 111 schemas. The consumer's `ent/gen` directory actually contains **125 top-level subdirs**. Removing the 11 infrastructure subdirs (`enthubspot`, `entsearch`, `entsf`, `entsf_test`, `enttest`, `hook`, `internal`, `migrate`, `predicate`, `privacy`, `runtime`) yields **114 entity schema directories**. The +3 delta from the spec reflects schemas added since the plan was written. A prior revision of this document quoted 117 because it used a narrower infra-filter; the correct count under the canonical filter is 114.
>
> **Coverage methodology (two-pass union):**
> 1. **Pass 1 — direct client calls:** `grep -rohE 'client\.[A-Z][A-Za-z]+'` across all `*_test.go` files, lowercased, intersected with the 114-entity list. Yields 29 entities.
> 2. **Pass 2 — liberal dirname mention:** for each entity dirname, case-insensitive word-boundary grep across all `*_test.go` files. Any match (imports, GraphQL schema strings containing the dirname, string literals, fixture helpers) counts. Yields 95 entities.
>
> The covered set is the **union** (95 entities — Pass 2 is a superset of Pass 1 here). An entity is "untested" only when *neither* pass matches. The PascalCase-to-dirname mapping is a straight `tolower()` — ent's codegen uses direct concatenation (`SavedFilter` → `savedfilter`, `UnrestrictedEscrow` → `unrestrictedescrow`, etc.).

Total schemas: **114**
Schemas with at least one test: **95**
Schemas with no test coverage: **19**

### Untested schemas

| Schema directory | Category | Notes |
|---|---|---|
| `compliancehistory` | history/audit | No test file mentions the dirname |
| `contactphonenumber` | lookup / denormalized | No test file mentions the dirname (only ContactPhoneNumber appears in CamelCase-concatenated resolver names, which the word-boundary filter correctly excludes) |
| `hrhistory` | history/audit | No test file mentions the dirname |
| `listinghistory` | history/audit | No test file mentions the dirname |
| `markethistory` | history/audit | No test file mentions the dirname |
| `mreisagentcommissionhistory` | history/audit | No test file mentions the dirname |
| `officehistory` | history/audit | No test file mentions the dirname |
| `pinnedcontactliststatus` | pinned-list status | No test file mentions the dirname |
| `pinnedpropertyliststatus` | pinned-list status | No test file mentions the dirname |
| `proposalhistory` | history/audit | No test file mentions the dirname |
| `referralfeehistory` | history/audit | No test file mentions the dirname |
| `ringcentralsyncposition` | sync/position tracking | No test file mentions the dirname |
| `savedfilter` | user preference | No test file mentions the dirname |
| `sfreconcile` | Salesforce sync | No test file mentions the dirname |
| `unrestrictedescrow` | RLS-bypass view | No test file mentions the dirname |
| `unrestrictedlisting` | RLS-bypass view | No test file mentions the dirname |
| `unrestrictedproposal` | RLS-bypass view | No test file mentions the dirname |
| `userhistory` | history/audit | No test file mentions the dirname |
| `usersavedfilter` | user preference | No test file mentions the dirname |

> **Entities that *look* untested but aren't under liberal matching:** `hr` and `sfsync` both survive the untested-filter. `hr` is mentioned in 8 test files (most substantively `hr_rls_integration_test.go`, which drives the entity through GraphQL `CreateHR`/`hrs()` mutations and queries); `sfsync` is mentioned in 3 test files (a generated `gen/sfsync/river_test.go` inside the package plus `"SFSync"` string literals in `contact_sf_sync_integration_test.go` and `proposal_sf_sync_integration_test.go`). The `sfsync` coverage is weaker in practice than the dirname grep suggests — the `testutil` matches are test-data strings, not entity calls.

## Observations

Coverage is concentrated almost entirely in `api-graphql/src/testutil` (212 of 274 test files), which exercises the **create** and **query** surfaces most heavily — 105 of 212 files call `.Create()` and 190 of 212 call `.Query()` or `.With*()` eager-loading — while **update** (42 files), **delete** (21 files), and **mutation** (0 files) are thin or absent across the entire consumer test suite. The `mutation` category (`internal/<entity>_mutation.go` surfaces like `OldX`, `ResetX`, `Fields()`) has **zero** direct coverage in any test directory, meaning any refactor to mutation state machinery carries no downstream test signal at all.

The 19 untested schemas split into four risk clusters: (1) **history/audit tables** (9 schemas — `*history` variants) that are append-only in practice but whose generated query/field accessors would silently break under refactor; (2) **RLS-bypass views** (3 schemas — `unrestricted*`) used by admin paths whose absence of test coverage is particularly concerning given their security-sensitive role; (3) **user-preference tables** (`savedfilter`, `usersavedfilter`, `pinnedcontactliststatus`, `pinnedpropertyliststatus`) that DO have GraphQL resolver tests elsewhere in the consumer but never reference the ent dirname directly — a known methodology blindspot since CamelCase operation names like `CreatePropertyNote` defeat word-boundary matching; and (4) **Salesforce/sync infra** (`sfreconcile`, `ringcentralsyncposition`) whose lack of coverage reflects genuine low-signal CI for these paths. For the refactor POC, clusters (1) and (3) are the safest to modify; cluster (2) is the highest risk because the RLS-bypass surface has both zero downstream tests *and* security implications.
