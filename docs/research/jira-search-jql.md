# Research: Jira Cloud `GET /rest/api/3/search/jql` per Atlassian's documentation (wayfinder #79)

**Question.** What does Jira Cloud's `GET /rest/api/3/search/jql` actually take and return, per Atlassian's own documentation, so the curl probe in `docs/jira-api/spike.sh` and the Go wire types can be written from the documented contract rather than from memory?

**Inspected 2026-10-14.** Short version: the request is `GET /rest/api/3/search/jql?jql=...&fields=summary&maxResults=50`; the response is `{ "issues": [IssueBean...], "isLast": bool, "nextPageToken": string|absent }` with **no `total`**; `fields` defaults to `id` (not `*navigable` as on the old `/search`), so `fields=summary` is required to get the summary at all; `maxResults` is documented as "max 5000"; pagination is a continuation token (`nextPageToken`, expires in 7 days), not `startAt`; the old `GET`/`POST /rest/api/3/search` is marked "Endpoint is currently being removed" ([CHANGE-2046](https://developer.atlassian.com/changelog/#CHANGE-2046), announced 2024-10-31, removal effective 2025-05-01, observed returning HTTP 410 by August 2025); counts moved to `POST /rest/api/3/search/approximate-count`.

**How this was sourced (read this first).** From this session every `*.atlassian.com` host (`developer.`, `support.`, `confluence.`, `community.`, `jira.`, `dac-static.`) was refused by the egress proxy, so the reference pages could not be fetched directly. The claims below about the REST contract are instead taken from **Atlassian's own OpenAPI document** (`swagger-v3.v3.json`, the file the reference site renders), read through two verbatim carriers:

- **Current (spec `_v=1.8516.116`, 2026-09):** [kubis1982/Atlassian.RestClient](https://github.com/kubis1982/Atlassian.RestClient) at commit `3c426c6` (2026-09-18) is a Microsoft Kiota client generated from `https://dac-static.atlassian.com/cloud/jira/platform/swagger-v3.v3.json?_v=1.8516.116` ([README, "OpenAPI Specifications"](https://github.com/kubis1982/Atlassian.RestClient/blob/3c426c630edb08c7d3953434232ed76379f48435/README.md)). Kiota copies each operation's, parameter's and schema property's `description` into `/// <summary>` doc comments unchanged. The quotes below come from [`JqlRequestBuilder.cs`](https://github.com/kubis1982/Atlassian.RestClient/blob/3c426c630edb08c7d3953434232ed76379f48435/src/Jira.V3/RestClient/Rest/Api/Three/Search/Jql/JqlRequestBuilder.cs), [`SearchRequestBuilder.cs`](https://github.com/kubis1982/Atlassian.RestClient/blob/3c426c630edb08c7d3953434232ed76379f48435/src/Jira.V3/RestClient/Rest/Api/Three/Search/SearchRequestBuilder.cs), [`ApproximateCountRequestBuilder.cs`](https://github.com/kubis1982/Atlassian.RestClient/blob/3c426c630edb08c7d3953434232ed76379f48435/src/Jira.V3/RestClient/Rest/Api/Three/Search/ApproximateCount/ApproximateCountRequestBuilder.cs) and the models [`SearchAndReconcileResults.cs`](https://github.com/kubis1982/Atlassian.RestClient/blob/3c426c630edb08c7d3953434232ed76379f48435/src/Jira.V3/RestClient/Models/SearchAndReconcileResults.cs), [`SearchAndReconcileRequestBean.cs`](https://github.com/kubis1982/Atlassian.RestClient/blob/3c426c630edb08c7d3953434232ed76379f48435/src/Jira.V3/RestClient/Models/SearchAndReconcileRequestBean.cs), [`JQLCountRequestBean.cs`](https://github.com/kubis1982/Atlassian.RestClient/blob/3c426c630edb08c7d3953434232ed76379f48435/src/Jira.V3/RestClient/Models/JQLCountRequestBean.cs), [`JQLCountResultsBean.cs`](https://github.com/kubis1982/Atlassian.RestClient/blob/3c426c630edb08c7d3953434232ed76379f48435/src/Jira.V3/RestClient/Models/JQLCountResultsBean.cs), [`ErrorCollection.cs`](https://github.com/kubis1982/Atlassian.RestClient/blob/3c426c630edb08c7d3953434232ed76379f48435/src/Jira.V3/RestClient/Models/ErrorCollection.cs), [`IssueBean.cs`](https://github.com/kubis1982/Atlassian.RestClient/blob/3c426c630edb08c7d3953434232ed76379f48435/src/Jira.V3/RestClient/Models/IssueBean.cs). Cited below as **[spec 1.8516]**. Kiota does *not* carry `default:`/`maximum:` schema keywords, response descriptions or examples, so those are flagged separately.
- **Old (2023-04 snapshot of the same file, pre-dating `/search/jql`):** [APIs.guru's copy](https://github.com/APIs-guru/openapi-directory/blob/main/APIs/atlassian.com/jira/1001.0.0-SNAPSHOT/openapi.yaml) (`x-origin: developer.atlassian.com/cloud/jira/platform/swagger-v3.v3.json`, last updated 2023-04-23). Used only for the retired `startAt` contract. Cited as **[spec 2023]**.
- The rendered page these describe is [Jira Cloud platform REST API v3, "Issue search"](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issue-search/) (anchors `#api-rest-api-3-search-jql-get`, `#api-rest-api-3-search-jql-post`, `#api-rest-api-3-search-approximate-count-post`). Cited as **[ref]** when a claim is about what the page shows but was only seen via search-engine excerpts.
- JQL facts come from search-engine excerpts of [JQL functions](https://support.atlassian.com/jira-software-cloud/docs/jql-functions/), [JQL keywords](https://support.atlassian.com/jira-software-cloud/docs/jql-keywords/) and [JQL operators](https://support.atlassian.com/jira-software-cloud/docs/jql-operators/) on support.atlassian.com, plus the older Server/DC [keywords reference](https://confluence.atlassian.com/display/JIRACORESERVER/Advanced+searching+-+keywords+reference). Marked **[excerpt]**.
- Deprecation text comes from the spec itself plus GitHub issues that reproduce Atlassian's CHANGE-2046 notice and the live removal error. Marked **[quoted in ...]**.

Anything not traceable to one of those is marked **[not verified]**.

---

## 1. The request

`GET /rest/api/3/search/jql`, listed on the reference page under the Issue search group as the enhanced JQL search (GET) [ref]. Operation description [spec 1.8516]:

> Searches for issues using JQL. Recent updates might not be immediately visible in the returned search results. If you need read-after-write consistency, you can utilize the `reconcileIssues` parameter to ensure stronger consistency assurances. This operation can be accessed anonymously. If the JQL query expression is too large to be encoded as a query parameter, use the POST version of this resource. **Permissions required:** Issues are included in the response where the user has: *Browse projects* project permission for the project containing the issue. If issue-level security is configured, issue-level security permission to view the issue.

Query parameters, with Atlassian's description verbatim [spec 1.8516]:

| Parameter | Type | Documented description |
|---|---|---|
| `jql` | string | "A JQL expression. For performance reasons, this parameter requires a bounded query. A bounded query is a query with a search restriction. * Example of an unbounded query: `order by key desc`. * Example of a bounded query: `assignee = currentUser() order by key`. Additionally, `orderBy` clause can contain a maximum of 7 fields." |
| `nextPageToken` | string | "The token for a page to fetch that is not the first page. The first page has a `nextPageToken` of `null`. Use the `nextPageToken` to fetch the next page of issues. Note: The `nextPageToken` field is **not included** in the response for the last page, indicating there is no next page." |
| `maxResults` | int32 | "The maximum number of items to return per page. To manage page size, API may return fewer items per page where a large number of fields or properties are requested. The greatest number of items returned per page is achieved when requesting `id` or `key` only. It returns max 5000 issues." |
| `fields` | string[] (comma-separated, repeatable) | "A list of fields to return for each issue, use it to retrieve a subset of fields. This parameter accepts a comma-separated list. Expand options include: * `*all` Returns all fields. * `*navigable` Returns navigable fields. * `id` Returns only issue IDs. * Any issue field, prefixed with a minus to exclude. The default is `id`. Examples: * `summary,comment` Returns only the summary and comments fields only. * `-description` Returns all navigable (default) fields except description. * `*all,-comment` Returns all fields except comments. Multiple `fields` parameters can be included in a request. Note: By default, this resource returns IDs only. This differs from GET issue where the default is all fields." |
| `expand` | string | "Use expand to include additional information about issues in the response. Note that, unlike the majority of instances where `expand` is specified, `expand` is defined as a comma-delimited string of values. The expand options are: `renderedFields`, `names`, `schema`, `transitions`, `operations`, `editmeta`, `changelog`, `versionedRepresentations` ... Examples: `"names,changelog"`" |
| `properties` | string[] | "A list of up to 5 issue properties to include in the results. This parameter accepts a comma-separated list." |
| `fieldsByKeys` | bool | "Reference fields by their key (rather than ID). The default is `false`." |
| `failFast` | bool | "Fail this request early if we can't retrieve all field data." |
| `reconcileIssues` | int64[] | "Strong consistency issue ids to be reconciled with search results. Accepts max 50 ids. This list of ids should be consistent with each paginated request across different pages." |
| `includeArchivedProjects` | bool | "Whether to also return issues that belong to archived projects. Issues in archived projects are excluded by default. Setting this to `true` returns them alongside issues from active projects; the *Browse projects* permission is still required on the archived project. The default is `false`." (New since the ticket was written; not in the 2024-era descriptions quoted by third parties.) |

**Does `fields=summary` limit the payload?** Yes, and more than on the old endpoint: the documented default is `id`, so without `fields` you get **only ids**, and `summary,comment` "Returns only the summary and comments fields only" [spec 1.8516]. On the retired `/search` the default was `*navigable` ("All navigable fields are returned by default") [spec 2023], which is why that endpoint's responses were large unless trimmed.

**Ceiling on `maxResults`.** The only documented number is in the description: "It returns max 5000 issues", qualified by "API may return fewer items per page where a large number of fields or properties are requested" [spec 1.8516]. The spec carries no `maximum:` keyword that this pass could see (Kiota drops it) **[not verified]**. The **default** is shown as `50` on the rendered page per third-party transcriptions ([thought-machine/go-jira PR #15](https://github.com/thought-machine/go-jira/pull/15/files), which copies the reference text and adds "Default: 50") **[not verified against Atlassian directly]**. Community reports say requesting `maxResults=0` is rejected with a message that it must be between 1 and 5000 ([Atlassian Community, "Get total on search API"](https://community.atlassian.com/forums/Jira-questions/Get-total-on-search-API-for-issues-using-JQL/qaq-p/3115617), via excerpt) **[not verified]**.

**Auth.** Same as the rest of the v3 API: `basicAuth` (email + API token) or OAuth2 `read:jira-work`, and anonymous is allowed ("This operation can be accessed anonymously") [spec 1.8516; security block seen in spec 2023 for the old operation]. Nothing changes for the spike's `-u "$JIRA_EMAIL:$JIRA_TOKEN"`.

## 2. The response

The 200 body is schema `SearchAndReconcileResults`, "The result of a JQL search with issues reconsilation [sic]" [spec 1.8516]. Its properties, verbatim:

| Property | Type | Documented description |
|---|---|---|
| `issues` | `IssueBean[]` | "The list of issues found by the search or reconsiliation." |
| `isLast` | bool | "Indicates whether this is the last page of the paginated response." |
| `nextPageToken` | string | "Continuation token to fetch the next page. If this result represents the last or the only page this token will be null. This token will expire in 7 days." |
| `names` | map[string]string | "The ID and name of each field in the search results." (only with `expand=names`) |
| `schema` | map[string]JsonTypeBean | "The schema describing the field types in the search results." (only with `expand=schema`) |
| `warnings` | (experimental) | "Experimental. Warnings generated during the search, e.g. when a JQL clause exceeded its argument limit or when the result set was truncated due to an ingestion limit. This field is currently rolling out behind a feature flag and may be absent, empty, or change shape without notice until generally available." |

**There is no `total`, no `startAt`, no `maxResults` echo** in the schema [spec 1.8516]. Compare the retired `SearchResults`: `expand`, `issues`, `maxResults`, `names`, `schema`, `startAt`, `total` ("The number of results on the page"), `warningMessages` [spec 2023].

Note the wording conflict on the last page: the parameter doc says the token "is **not included** in the response for the last page", the schema doc says it "will be null". Both are in the same spec; a parser must accept **absent or `null`** as "no more pages". `isLast` was added later than the token (Atlassian tracked it as [JRACLOUD-94648 "Add isLast to response for JQL enhanced search"](https://jira.atlassian.com/browse/JRACLOUD-94648), seen via excerpt) and several integrators report it misbehaving ([atlassian/atlassian-mcp-server#118](https://github.com/atlassian/atlassian-mcp-server/issues/118), [pycontribs/jira#2379](https://github.com/pycontribs/jira/issues/2379) comment "it does not set isLast correctly"), so do not make termination depend on `isLast` alone.

**Per-issue shape.** Each element is the ordinary `IssueBean` ("Details about an issue"): `id` "The ID of the issue", `key` "The key of the issue", `self` "The URL of the issue details", `expand`, `fields` (free-form object keyed by field id), plus the optional `renderedFields`, `names`, `schema`, `transitions`, `operations`, `editmeta`, `changelog`, `versionedRepresentations`, `properties` [spec 1.8516]. With `fields=summary` the `fields` object holds just `summary` (a string). This is the same bean the repo's v1 spike already captured from `GET /issue/{key}?fields=summary` in `docs/jira-api/responses/issue-summary.json`:

```json
{"expand":"renderedFields,names,schema,operations,editmeta,changelog,versionedRepresentations","id":"100001","self":"https://example.atlassian.net/rest/api/3/issue/100001","key":"EX-1234","fields":{"summary":"Example ticket summary for fixture purposes"}}
```

So the expected search body is, in shape:

```json
{
  "issues": [
    { "id": "100001", "self": "https://example.atlassian.net/rest/api/3/issue/100001", "key": "EX-1234",
      "fields": { "summary": "..." } }
  ],
  "isLast": true
}
```

The reference page's worked example JSON could not be fetched in this pass **[not verified]**; the shape above is derived from the schema plus the repo's own fixture.

## 3. Pagination

**Old model (retired `GET /search`)** [spec 2023]: `startAt` "The index of the first item to return in a page of results (page offset)", default 0; `maxResults` default 50, "To manage page size, Jira may return fewer items per page where a large number of fields are requested"; response echoes `startAt`, `maxResults`, `total`. Random access by offset; `total` available on every page. (The 100-per-page cap that Cloud enforced on this endpoint is not in the spec; community posts report it **[not verified]**.)

**New model (`/search/jql`)** [spec 1.8516]: an opaque continuation token. First request sends no `nextPageToken` ("The first page has a `nextPageToken` of `null`"); each page returns `nextPageToken` until the last, where it is absent/null and `isLast` is true; tokens "expire in 7 days". No offset, so no random page access, and no `total`. Atlassian's own KB phrases it as "pagination for this endpoint now relies on the nextPageToken parameter instead of the deprecated startAt parameter" and "Random page access is no longer supported" ([Run JQL search query using Jira Cloud REST API](https://confluence.atlassian.com/jirakb/run-jql-search-query-using-jira-cloud-rest-api-1289424308.html), via excerpt). Atlassian also documents that the result set is not a stable snapshot across pages ([JRACLOUD-95368](https://jira.atlassian.com/browse/JRACLOUD-95368), via excerpt) and that the endpoint is eventually consistent, "The majority of modifications are shown within seconds", with `reconcileIssues` (max 50 ids) as the read-after-write escape hatch ([Search and reconcile](https://developer.atlassian.com/cloud/jira/platform/search-and-reconcile/), via excerpt).

**"The first fifty, no more."** Send `maxResults=50` and no `nextPageToken`, then **ignore** `nextPageToken` and `isLast` in the response. Nothing else is needed: the server does not pre-fetch further pages, and the token is only meaningful if you send it back. Two caveats from the docs: (a) "fewer items per page" is allowed, so treat it as *up to* fifty, and with `fields=summary` only that is unlikely to bite; (b) a bounded JQL is mandatory, so the query must contain at least one restriction, which `assignee = currentUser()` satisfies (it is literally Atlassian's example of a bounded query).

## 4. Errors

**Documented responses for `GET /search/jql`.** The reference page lists 200, 400 and 401 [ref]. The response descriptions themselves were not captured in this pass (Kiota drops them) **[not verified]**; for the retired `/search` they were 400 "Returned if the JQL query is invalid." and 401 "Returned if the authentication credentials are incorrect or missing." [spec 2023], and the `jql` description's "bounded query" requirement is the one new 400 cause. Notably the spec declares **no error schema** for these responses on either endpoint (Kiota generated no `errorMapping`), i.e. the body on error is not formally specified per-operation.

**The error envelope.** Jira's generic error schema is `ErrorCollection`, "Error messages from an operation.": `errorMessages` string[] "The list of error messages produced by this operation. For example, 'input parameter 'key' must be provided'"; `errors` map[string]string "The list of errors by parameter returned by the operation. For example, 'projectKey': 'Project keys must start with an uppercase letter...'"; `status` int32 [spec 1.8516; identical in spec 2023]. This is exactly what the v1 spike recorded for a 404 in `docs/jira-api/responses/issue-404.json`:

```json
{"errorMessages":["Issue does not exist or you do not have permission to see it."],"errors":{}}
```

**Invalid JQL (expected 400).** Expect `ErrorCollection` with the parser message in `errorMessages` (the retired endpoint documented: "If the JQL is not correctly formed a 400 response code is returned, regardless of the `validateQuery` value" [spec 2023]; `/search/jql` has no `validateQuery` parameter at all [spec 1.8516], so there is no "warn" mode to turn this off). The exact message text for a syntax error is not documented; the probe should capture it **[not verified]**.

**401.** Basic auth with a bad token: Jira returns 401 and, for REST calls, the usual `ErrorCollection`-style body or an empty body with `WWW-Authenticate`; this pass could not confirm which from a primary source **[not verified]**, so capture it.

**404 on this endpoint** is not listed [ref]: an issue key that does not exist is just a JQL that matches nothing (200, `issues: []`, `isLast: true`), and a key in a project you cannot see is filtered by the permission rule in the operation description. 404 from the *old* path is now a **410**: the removed endpoint answers `HTTP 410` with body `The requested API has been removed. Please use the newer, enhanced search-based API instead.` (observed 2025-08-21, [quoted in onyx-dot-app/onyx#5235](https://github.com/onyx-dot-app/onyx/issues/5235)) or `The requested API has been removed. Please migrate to the /rest/api/3/search/jql API.` ([quoted in pycontribs/jira#2379](https://github.com/pycontribs/jira/issues/2379), 2025-09). That body is plain text, **not** the JSON envelope, which is itself a reason to key the client on status code first and JSON second.

## 5. Deprecation of `GET`/`POST /rest/api/3/search`

- **In the spec today** [spec 1.8516], both the GET and POST `/rest/api/3/search` descriptions begin: "Endpoint is currently being removed. [More details](https://developer.atlassian.com/changelog/#CHANGE-2046)". The v2 twins (`/rest/api/2/search`) carry the same banner. The operations still exist in the document (so generated clients still compile) but the live service answers 410 (section 4).
- **The notice.** Atlassian's changelog entry [CHANGE-2046](https://developer.atlassian.com/changelog/#CHANGE-2046) (page not fetchable here) is reproduced in [pycontribs/jira#1940](https://github.com/pycontribs/jira/issues/1940) and [jenkinsci/jira-plugin#636](https://github.com/jenkinsci/jira-plugin/issues/636): "On October 31, 2024, Atlassian announced the deprecation of four Jira Cloud search APIs" ... "Effective May 1, 2025, Atlassian will remove the following Jira REST endpoints: `GET /rest/api/3/search`, `POST /rest/api/3/search`, `POST /rest/api/3/search/id`, `POST /rest/api/3/expression/eval`" ... "These APIs will be replaced with new, more efficient alternatives through our enhanced JQL service". An Atlassian employee reiterated "we're soon approaching the deadline of 1st May" on 2025-04-17 in the Jenkins thread. Several community posts say the cut-over was later rolled out gradually and cite 2025-08-01 as the enforcement date; that date traces to ScriptRunner's release notes, not to Atlassian **[not verified]**. By 2025-08-21 production tenants were returning 410 (section 4), and Atlassian's KB now states the legacy endpoint "has been fully removed from Jira Cloud" and to "migrate all uses of this endpoint to /rest/api/3/search/jql" ([KB](https://confluence.atlassian.com/jirakb/run-jql-search-query-using-jira-cloud-rest-api-1289424308.html), via excerpt, last modified 2025-12-05).
- **Why the new design.** Atlassian's [RFC-61 "Evolving Search Capabilities: Addressing Scalability with a New, Enhanced Search API"](https://community.developer.atlassian.com/t/rfc-61-evolving-search-capabilities-addressing-scalability-with-a-new-enhanced-search-api/83027) (2024, via excerpt) describes the change as replacing random page access with a continuation-token API, dropping `total` in favour of `POST /rest/api/3/search/approximate-count`, and making search eventually consistent by default.
- **Conclusion for the decision record:** `/search/jql` is not a preference, it is the only JQL search endpoint Jira Cloud serves. The reason to record is "the `/search` family was removed under CHANGE-2046 (announced 2024-10-31, effective 2025-05-01, 410 in production by 2025-08)".

## 6. JQL: `currentUser()`, `resolution = EMPTY`, default order

- **`currentUser()`** [excerpt, JQL functions]: searches relative to the currently logged-in user. Supported fields: Assignee, Reporter, Voter, Watcher, Creator, custom fields of type User. Supported operators: `=`, `!=` only (not `~`, `>`, `IN`, `WAS`, `CHANGED`). "this function can only be used by logged-in users", so don't use it in filters shared with anonymous users. Canonical example: `assignee = currentUser()`. Independently, Atlassian uses `assignee = currentUser() order by key` as its example of a bounded query in the `/search/jql` `jql` parameter description [spec 1.8516], which doubles as confirmation that the function parses on this endpoint.
- **`EMPTY`** [excerpt, JQL keywords]: "Used to search for work items where a given field does not have a value" ... "EMPTY can only be used with fields that support the IS and IS NOT operators"; `NULL` is a synonym. The Server/DC keywords reference gives both spellings as equivalent: "duedate = empty" or "duedate is empty" ([keywords reference](https://confluence.atlassian.com/display/JIRACORESERVER/Advanced+searching+-+keywords+reference), via excerpt). Atlassian's own example query uses the `is` form: "duedate < now() and resolution is empty" [excerpt, JQL keywords]. `resolution = EMPTY` is therefore valid syntax, but there is an open Cloud bug about `= EMPTY` inside compound `OR` clauses ([JRACLOUD-67419](https://jira.atlassian.com/browse/JRACLOUD-67419), via excerpt), so prefer **`resolution IS EMPTY`**, which is the operator the keyword is defined for.
- **Default sort when `ORDER BY` is omitted.** The keywords docs define `ORDER BY` as "Used to specify the fields by whose values the search results will be sorted. By default, the field's own sorting order will be used. You can override this by specifying ascending order ("asc") or descending order ("desc")" and require it "at the end of the JQL query" [excerpt, JQL keywords]. That sentence is about the *direction* when a field is named; **Atlassian does not document what order the REST search returns when there is no `ORDER BY` at all** **[not verified / not documented]**. Community observations say "issue key descending" but that is anecdote. Punchcard should therefore always send an explicit `ORDER BY` (e.g. `ORDER BY updated DESC`), which is also what makes `nextPageToken` pages meaningful. The clause is limited to 7 fields [spec 1.8516].

## 7. Also documented: `POST /search/jql` and `approximate-count`

- **`POST /rest/api/3/search/jql`** exists (the POST twin on the reference page) [ref], for when "the JQL query expression is too large to be encoded as a query parameter" [spec 1.8516]. Body schema `SearchAndReconcileRequestBean` with the same members as the GET query string: `jql`, `nextPageToken`, `maxResults`, `fields` (string[] , "The default is `id`"), `expand` (string), `properties`, `fieldsByKeys`, `reconcileIssues`, `includeArchivedProjects` [spec 1.8516]. Same `SearchAndReconcileResults` response. Not needed for Punchcard's short query.
- **`POST /rest/api/3/search/approximate-count`**, "Count issues using JQL" [ref]: "Provide an estimated count of the issues that match the JQL. Recent updates might not be immediately visible in the returned output. This endpoint requires JQL to be bounded. This operation can be accessed anonymously." Request body `JQLCountRequestBean` `{ "jql": "..." }` ("For performance reasons, this parameter requires a bounded query"); response `JQLCountResultsBean` `{ "count": <int64> }` "Number of issues matching JQL query." [spec 1.8516]. It is POST-only (no GET variant is generated). Atlassian's KB example: `curl ... -X POST .../rest/api/3/search/approximate-count --data '{ "jql": "<insert your full JQL query here>" }'`, response `{ "count": 153 }` ([Find the total number of work items](https://support.atlassian.com/jira/kb/find-the-total-number-of-work-items-in-jira-cloud-using-jql-or-issue-search/), via excerpt). Punchcard does not need a count for "my open tickets, first fifty", but this is the endpoint if a badge ever wants one.

## 8. Not confirmed from a primary source in this pass

1. The literal `Default: 50` for `maxResults` on `/search/jql` and any `maximum:` keyword in the schema (only the prose "max 5000 issues" was seen verbatim).
2. The 400/401 response *descriptions* and the example 200 JSON on the `/search/jql` reference page.
3. The exact body of a 401 from Basic auth on this endpoint, and the exact `errorMessages` text for a JQL syntax error.
4. The text of changelog entry CHANGE-2046 itself and whether Atlassian ever formally moved the date from 2025-05-01 to 2025-08-01 (the later date is third-party).
5. Any documented default sort for a JQL query with no `ORDER BY` (appears to be undocumented, not merely unfetched).
6. The exact Cloud page-size cap on the *old* endpoint (100 per page is widely reported but is not in the spec).

All of 1-3 are exactly what the spike probe below will record as fixtures.

---

## What this means for Punchcard

**The probe to add to `docs/jira-api/spike.sh`** (same `probe` helper, same `AUTH`/`JSON` arrays; `--data-urlencode` so `=`, spaces and parentheses in the JQL are encoded by curl, and `-G` so the data goes on the query string of a GET):

```bash
# ------------------------------------------------------------ 5. JQL search
echo "== 5. my open tickets via /search/jql =="
MY_JQL='assignee = currentUser() AND resolution IS EMPTY ORDER BY updated DESC'
probe search-mine -G \
  --data-urlencode "jql=$MY_JQL" \
  --data-urlencode "fields=summary" \
  --data-urlencode "maxResults=50" \
  "$BASE/rest/api/3/search/jql"
# expect: 200, {"issues":[{id,key,self,fields:{summary}}...],"isLast":bool[,"nextPageToken"]}, no "total"

# invalid JQL: the 400 envelope
probe search-badjql -G \
  --data-urlencode "jql=assignee = currentUser( AND resolution IS EMPTY" \
  --data-urlencode "fields=summary" \
  "$BASE/rest/api/3/search/jql"

# unbounded JQL: /search/jql rejects queries with no restriction (documented), capture how
probe search-unbounded -G \
  --data-urlencode "jql=ORDER BY key DESC" \
  "$BASE/rest/api/3/search/jql"

# bad credentials: the 401 shape on this endpoint
curl -sS -o "$OUT/search-401.json" -w '%{http_code}\n' -u "$JIRA_EMAIL:not-a-token" "${JSON[@]}" -G \
  --data-urlencode "jql=$MY_JQL" --data-urlencode "fields=summary" \
  "$BASE/rest/api/3/search/jql"

# the retired endpoint, to record the 410 we are avoiding
probe search-legacy-410 -G --data-urlencode "jql=$MY_JQL" --data-urlencode "maxResults=1" \
  "$BASE/rest/api/3/search"
```

Then `redact.py` the new fixtures like the others (issue keys, summaries, `self` URLs, and the opaque `nextPageToken` if one appears; the token is tied to the tenant and expires in 7 days, so it must not be committed as-is).

**Go wire types** (minimal, decode-only, `encoding/json`; everything optional so an absent `nextPageToken` and the experimental `warnings` field are harmless):

```go
// SearchResponse is the 200 body of GET /rest/api/3/search/jql
// (schema SearchAndReconcileResults). There is no total.
type SearchResponse struct {
	Issues        []Issue `json:"issues"`
	IsLast        bool    `json:"isLast"`
	NextPageToken string  `json:"nextPageToken,omitempty"` // absent or null on the last page; expires in 7 days
}

// Issue is the IssueBean subset Punchcard reads. Fields is keyed by field id;
// with fields=summary it holds only summary.
type Issue struct {
	ID     string      `json:"id"`
	Key    string      `json:"key"`
	Self   string      `json:"self"`
	Fields IssueFields `json:"fields"`
}

type IssueFields struct {
	Summary string `json:"summary"`
}

// ErrorCollection is Jira's generic error envelope (400, 401, 404, ...).
type ErrorCollection struct {
	ErrorMessages []string          `json:"errorMessages"`
	Errors        map[string]string `json:"errors"`
	Status        int               `json:"status,omitempty"`
}
```

Request: `GET {base}/rest/api/3/search/jql` with `url.Values{"jql": {...}, "fields": {"summary"}, "maxResults": {"50"}}`, `Accept: application/json`, Basic auth. For "first fifty" never send `nextPageToken` and never loop. If pagination is ever needed: loop while `NextPageToken != ""`, send it back unchanged, stop also when `IsLast` is true or `Issues` is empty, and cap iterations, because of the reported `isLast` misbehaviour.

**Error cases to capture and handle:**

| Case | Status | Body | Client behaviour |
|---|---|---|---|
| Bad JQL syntax | 400 (documented) | `ErrorCollection`, message in `errorMessages[0]` (text to be recorded by the probe) | Surface `errorMessages` joined; it is a programming error in Punchcard's query |
| Unbounded JQL | 400 (documented requirement; status/body to be recorded) | `ErrorCollection` | Same as above; cannot happen with `assignee = currentUser()` |
| Bad/missing credentials | 401 (documented) | to be recorded (envelope or empty) | "Check your API token" path, same as `/myself` |
| Wrong endpoint (`/search`) | 410 (observed) | plain text, not JSON | Only reachable by regression; a JSON decode failure must not panic, so decode errors on non-2xx should fall back to the raw body |
| Zero matches | 200 | `{"issues":[],"isLast":true}` | Empty list, not an error |
| Permission-filtered issue | 200 | the issue is simply absent | Nothing to do |
| Stale read after a write | 200 | newly created/edited issue may be missing for seconds | Acceptable for a ticket picker; `reconcileIssues` exists if it ever matters |
