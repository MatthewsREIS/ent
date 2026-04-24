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
| `api-graphql/src/testutil` | 212 | ✓ | ✓ | ✓ | ✓ | | ✓ | ✓ | ✓ | AgentLicensing, AiChatMessage, AiChatThread, AlphamapMartOutput, BoxFolder, BrokerOfRecord, ChatterLastReadMessage, ChatterMention, ChatterMessage, ChatterNotification, ChatterThread, CoBroker, Collateral, Commission, Company, CompanyHistory, CompanyNote, Compliance, Contact, ContactHistory, ContactList, ContactListContact, ContactListShare, ContactNote, Content, Contract, CrexiMartOutput, DealNotification, DealTeam, DmEmailStats, Email, EmailStats, EmailToBrokersTask, Escrow, HtmlEmailTask, HyperionMartOutput, Intercept, Lease, Listing, Market, MarketAgentRegion, Marketing, MreisAgentCommission, Notification, Offer, Office, OfficeUser, Opportunity, Outreach, OutreachQueue, Ownership, PartyToTransaction, PhoneNumber, PinnedContactList, PinnedContactListStatus, PinnedPropertyList, PinnedPropertyListStatus, Profile, Property, PropertyFieldHistory, PropertyFieldSelection, PropertyList, PropertyNote, Proposal, PublicWebPostsTask, RecordType, ReferralFee, RelatesTo, RequestEmailTask, RequestPhotosTask, RingCentralPhoneNumber, RingCentralPhoneNumberMdt, RoleDefinition, SalesComp, SavedFilter, Signage, Space, Task, TextEmailTask, Timeline, User, UserContact, UserReminder, UserSavedFilter, UserSettings, UserTask, Website, WrikeAttachment, WrikeProject, WrikeTask, WrikeWorkflowStatus |
| `api-graphql/src/txutil` | 1 | | | | | | ✓ | | ✓ | gen.Tx interface testing only — no entity queries |
| `webhooks/hubspot` | 1 | | ✓ | | | | | | | Content |

## Schema coverage summary

> **Note on schema count:** The task spec cited 111 schemas. The consumer's `ent/gen` directory contains 117 schema subdirectories (118 dirs minus `entsf_test`, which is a test-package directory, not an entity schema). The discrepancy is likely due to schemas added since the spec was written.

Total schemas: **117**
Schemas with at least one test: **105**
Schemas with no test coverage: **12**

### Untested schemas

| Schema directory | Category | Notes |
|---|---|---|
| `compliancehistory` | history/audit | No test, no GraphQL reference |
| `deletedcampaignmember` | soft-delete record | No test, no GraphQL reference |
| `hrhistory` | history/audit | No test, no GraphQL reference |
| `listinghistory` | history/audit | No test, no GraphQL reference |
| `markethistory` | history/audit | No test, no GraphQL reference |
| `mreisagentcommissionhistory` | history/audit | No test, no GraphQL reference |
| `officehistory` | history/audit | No test, no GraphQL reference |
| `proposalhistory` | history/audit | No test, no GraphQL reference |
| `referralfeehistory` | history/audit | No test, no GraphQL reference |
| `ringcentralsyncposition` | sync/position tracking | No test, no GraphQL reference |
| `sfreconcile` | Salesforce sync | No test, no GraphQL reference |
| `userhistory` | history/audit | No test, no GraphQL reference |

## Observations

Coverage is concentrated almost entirely in `api-graphql/src/testutil` (212 of 274 test files), which exercises the **create** and **query** surfaces most heavily — 105 of 212 files call `.Create()` and 190 of 212 call `.Query()` or `.With*()` eager-loading — while **update** (42 files), **delete** (21 files), and **mutation** (0 files) are thin or absent across the entire consumer test suite. The `mutation` category (`internal/<entity>_mutation.go` surfaces like `OldX`, `ResetX`, `Fields()`) has **zero** direct coverage in any test directory, meaning any refactor to mutation state machinery carries no downstream test signal at all. The 12 untested schemas are overwhelmingly **history/audit tables** (9 of 12) plus three sync/tracking entities; these schemas are read-only append-only tables in practice, but a refactor to their generated query or field accessors would be invisible to CI until a production regression occurred.
