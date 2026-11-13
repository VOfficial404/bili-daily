# Live, relation, notifications, and configuration protocol spec

**Research baseline:** BiliBiliToolPro `main`, tree [`4ab02ce28cae854a3d84c713a3ccb483006b8213`](https://github.com/RayWangQvQ/BiliBiliToolPro/tree/4ab02ce28cae854a3d84c713a3ccb483006b8213). Protocol facts only; no source code copied.

**Source availability:** The requested [SocialSisterYi/bilibili-API-collect](https://github.com/SocialSisterYi/bilibili-API-collect) redirects to a repository containing a closure notice and no API documents. Both `master` and `main` recursive-tree requests failed. Historical relation details below are explicitly attributed to the preserved [`RayWangQvQ/bilibili-API-collect/user/relation.md`](https://github.com/RayWangQvQ/bilibili-API-collect/blob/93092f26348d33468fd4799ca4c84b9522c8ccab/user/relation.md) fork, dated by its repository metadata to 2021. Those historical facts do not establish current server behavior.

**Notification dependency:** The HTTP implementations live outside BiliBiliToolPro in `Ray.Serilog.Sinks.*`, pinned to `0.1.6` by [`Directory.Packages.props`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/Directory.Packages.props). NuGet metadata identifies source commit [`0acd4cd0ac50bcfea5b0ad2ac874ca15301b13c0`](https://github.com/RayWangQvQ/Ray.Serilog.Sinks/tree/0acd4cd0ac50bcfea5b0ad2ac874ca15301b13c0). Notification transport facts below use that exact commit.

## Shared request and result conventions

- Bilibili authenticated requests send the account Cookie string, including `SESSDATA`; mutation CSRF values come from Cookie `bili_jct`.
- All live, relation, and live-trace clients here use the **browser** `Security:UserAgent`, including endpoints whose path contains `app-ucenter`. They do not use `access_key`, appkey signatures, or the app UA.
- Headers listed below describe what BiliBiliToolPro sends. Whether every header is a server-side minimum requirement is **[UNCONFIRMED]**.
- Shared headers: `Accept: application/json, text/plain, */*`; browser-style `Accept-Language`; `Sec-Fetch-Dest: empty`, `Sec-Fetch-Mode: cors`, `Sec-Fetch-Site: same-site`, and `Connection: keep-alive`, unless an endpoint overrides them.
- POST forms use `application/x-www-form-urlencoded`; form/query field names are case-sensitive. The client normalizes the first letter of form keys to lowercase.
- Response envelope: `$.code`, `$.message`, optional `$.data`. Missing `data` on an error is valid. The client uses `code == 0` as success.
- Except where explicitly documented below, endpoint-specific “already participated,” “already completed today,” “not eligible,” `-352`, and `-412` mappings are **[UNCONFIRMED]**. The implementation usually logs the supplied message and treats any nonzero code as failure; do not assign invented meanings.

Sources: [`Agent/Extensions/ServiceCollectionExtension.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/Extensions/ServiceCollectionExtension.cs), [`BiliBiliCommonHeadersDelegatingHandler.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/HttpClientDelegatingHandlers/BiliBiliCommonHeadersDelegatingHandler.cs), [`FormUrlEncodedKeyNormalizingDelegatingHandler.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/HttpClientDelegatingHandlers/FormUrlEncodedKeyNormalizingDelegatingHandler.cs), [`BiliApiResponse.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/BiliApiResponse.cs).

## 1. `LiveLottery` — 天选时刻

### Numbered execution flow

1. Check `LiveLotteryTaskConfig:IsEnable`; disabled means return.
2. Ensure account cookies/device cookies exist; print logged-in user information.
3. If automatic grouping is enabled, fetch the newest followed creator and remember its UID.
4. Fetch parent live areas.
5. Scan **pages 1–3 per area**, each with `page_size=99`, `area_id=0`, sorting by online audience.
6. Keep only rooms whose `pendant_info["2"].pendent_id` is numeric **504 or 1432**.
7. For each candidate, apply UID denylist, fetch lottery details, and apply the eligibility filters below.
8. Submit the lottery Join form. Record successful rooms requiring follow.
9. After the scan, identify newly followed lottery hosts and copy them into the hard-coded group **`天选时刻`**.
10. End. **No immediate post-lottery unfollow occurs.** `UnfollowBatched` is a separate task and schedule.

Sources: [`Application/LiveLotteryTaskAppService.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Application/LiveLotteryTaskAppService.cs), [`DomainService/LiveDomainService.cs`, `TianXuan`, `TryJoinTianXuan`, `GroupFollowing`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/LiveDomainService.cs).

### Discovery and participation endpoints

All URLs in this table are full URLs; query parameters are listed separately.

| Step | METHOD / URL | Auth and endpoint headers | Query/body parameters | Relevant response fields and outcomes |
|---|---|---|---|---|
| Parent areas | **GET** `https://api.live.bilibili.com/xlive/web-interface/v1/index/getWebAreaList` | Cookie; browser UA. No endpoint-specific Referer/Origin declaration. | Query `source_id=2`. | `$.data.data[]`: `id`, `name`. Nonzero code or missing nested data aborts the scan. |
| Active room discovery | **GET** `https://api.live.bilibili.com/room/v3/area/getRoomList` | Cookie; browser UA; `Referer: https://live.bilibili.com/`; `Origin: https://live.bilibili.com`. **No WBI signature in this client.** | Query `platform=web`, `parent_area_id=<area.id>`, `area_id=0`, `sort_type=online`, `page=1..3`, `page_size=99`. | `$.data.list[]`: `roomid`, `uid`, `uname`, `title`, `pendant_info`; `$.data.has_more` exists in DTO but is not used for stopping. Stop pagination when list length `<99`; list error breaks that area’s pagination. |
| Retained alternative, currently unused by lottery flow | **GET** `https://api.live.bilibili.com/xlive/web-interface/v1/second/getList` | Cookie; browser UA; live Referer/Origin; **WBI** through `wts`/`w_rid`. | Query DTO: `platform`, `parent_area_id`, `area_id`, `sort_type`, `page`, `wts`, `w_rid`. | Same list envelope; also `$.data.new_tags[]`: `id`, `name`, `sort_type`. Interface comment explicitly says v3 discovery replaces this endpoint to avoid **risk-control `-352`**. Current flow does **not** discover/select a lottery tag from `new_tags`. |
| Lottery details | **GET** `https://api.live.bilibili.com/xlive/lottery-interface/v1/Anchor/Check` | Cookie; browser UA; live Referer/Origin; no WBI/app signing. | Query `roomid=<candidate roomid>`. | `$.data.id`, `room_id`, `status`, `award_name`, `award_num`, `danmu`, `join_type`, `require_type`, `require_value`, `require_text`, `gift_id`, `gift_num`, `gift_price`, `gift_name`, `cur_gift_num`, `send_gift_ensure`. `status=1` eligible/open; `2` ended. The implementation reads `.Data` without explicitly checking the Check response’s code. |
| Join | **POST** `https://api.live.bilibili.com/xlive/lottery-interface/v1/Anchor/Join` | Cookie + `bili_jct`; browser UA. No endpoint-specific Referer/Origin declaration. | URL-encoded body: `id=<Check.id>`, `gift_id=<Check.gift_id>`, `gift_num=<Check.gift_num>`, `platform=pc`, `csrf=<bili_jct>`, `csrf_token=<same>`, `visit_id=<process-generated value>`. No room ID is included by this DTO. | `code=0` means successful participation. DTO exposes `$.data.discount_id`, `gold`, `silver`, `cur_gift_num`, `goods_id`, `new_order_id`, but task logic only uses `code`/`message`. Nonzero: log failure and continue. Specific duplicate/eligibility codes **[UNCONFIRMED]**. |

Sources: [`Interfaces/ILiveApi.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Interfaces/ILiveApi.cs), [`AreaDto.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveApi/AreaDto.cs), [`ListItemDto.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveApi/ListItemDto.cs), [`GetRoomListV3Request.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveApi/GetRoomListV3Request.cs), [`GetListRequest.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveApi/GetListRequest.cs), [`CheckTianXuanDto.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveApi/CheckTianXuanDto.cs), [`JoinTianXuanRequest.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveApi/JoinTianXuanRequest.cs), [`JoinTianXuanResponse.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveApi/JoinTianXuanResponse.cs).

### Exact filters and follow behavior

| Condition | Implemented behavior |
|---|---|
| Pendant structure | Require an object containing object key `"2"` containing integer `pendent_id`; accept 504 or 1432. Null, arrays, scalars, missing fields, and noninteger IDs are skipped. |
| Host denylist | Skip if room `uid` matches `DenyUids`. |
| Lottery state | Require `status=1`. Missing lottery data is skipped. |
| Exclude keywords | Case-sensitive substring matching against `award_name`; **any exclusion wins**, including when an inclusion also matches. |
| Include keywords | Empty list allows all remaining names; otherwise require **at least one** inclusion substring. |
| Gifts | Skip if **`gift_price > 0`**. The implementation does not separately test `gift_num`, `gift_id`, or `send_gift_ensure`. |
| Follow requirement | Allow only `require_type=0` or `1`. `null` is rejected by the comparisons. Type `2` means medal-level requirement; `3` means guard-tier requirement; both skipped. |
| Actual following request | No separate `x/relation/modify act=1` call in this flow. It submits Join and, on success with `require_type=1`, tracks the host as a potential new follow. Actual server-side automatic-follow semantics are **[UNCONFIRMED]** beyond this client assumption. |
| Danmaku requirement | `danmu` and `join_type` are represented, but **not inspected or acted upon**. No separate lottery danmaku is sent. |
| Red packets | No separate red-packet detection, Join endpoint, or amount filter. A prize containing `红包` is handled by ordinary award-name matching. |
| Exceptions | Each room’s participation logic catches exceptions, logs a warning, and continues. |

`visit_id`: generated once for the request class, consisting of a random first digit 1–9, ten lowercased generated characters, and final `0`; it is reused across joins in that process.

Source: [`LiveDomainService.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/LiveDomainService.cs), [`CheckTianXuanDto.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveApi/CheckTianXuanDto.cs), [`JoinTianXuanRequest.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveApi/JoinTianXuanRequest.cs).

### Automatic grouping

1. Before scanning, call followings with time-descending order and remember the first UID.
2. After scanning, fetch **only the first page, 20 entries**, again.
3. Collect entries before the remembered UID; intersect with hosts whose `require_type=1` lotteries were successfully joined.
4. Query tags and find exact group name `天选时刻`; create it if absent.
5. Batch **copy** hosts to that group with `tags/copyUsers`. Despite comments describing transfer/movement, the actual endpoint is **copy**, not `moveUsers`.
6. Existing follows are excluded by the before/after intersection. More than 20 new follows can be missed because the grouping query does not paginate.
7. If group creation fails, the helper returns group ID `0`; the caller still constructs its copy request with that ID.

Relation endpoints, fields, headers, and historical codes are specified in §3.

Source: [`LiveDomainService.cs`, `GetLastFollowUpId`, `GetNeedGroup`, `GetOrCreateTianXuanGroupId`, `GroupFollowing`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/LiveDomainService.cs).

### Configuration and limits

All keys below are under `LiveLotteryTaskConfig`.

| Key | Console/Web default | Meaning |
|---|---|---|
| `IsEnable` | `true` | Enable task. |
| `Cron` | `0 0 22 * * ?` | Daily 22:00 in Web scheduler. |
| `ExcludeAwardNames` | `舰\|船\|航海\|代金券\|自拍\|照\|写真\|图\|提督` | Pipe-separated exclusion substrings. |
| `IncludeAwardNames` | `""` | Pipe-separated inclusion substrings. |
| `AutoGroupFollowings` | `true` | Group newly followed lottery hosts. |
| `DenyUids` | `65566781,1277481241,1643654862,603676925` | Comma-separated host UID denylist. |

Keyword/UID parsing trims entries and removes empty entries. Area/page limits and pendant IDs are hard-coded, not configuration knobs. There is no maximum participation count, “skip follow-required,” lottery-danmaku, red-packet, or immediate-unfollow switch in this options class.

When API spacing is enabled and applies to POST, Join has a fixed **3-second pre-request delay**. See §5 for general spacing.

Sources: [`LiveLotteryTaskOptions.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Config/Options/LiveLotteryTaskOptions.cs), [`Web/appsettings.json`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Web/appsettings.json), [`IntervalDelegatingHandler.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/HttpClientDelegatingHandlers/IntervalDelegatingHandler.cs).

## 2. `LiveFansMedal`

### Numbered execution flow

1. Check `LiveFansMedalTaskConfig:IsEnable`; ensure account cookies.
2. Run **danmaku → likes → watch heartbeat**, in that order. Each action has an independent enable flag.
3. Before each action, ensure `LIVE_BUVID`; if missing, obtain it through the live-cookie bootstrap endpoint.
4. Fetch the current paginated medal **panel**, combine normal and special lists, deduplicate by anchor UID, and apply anchor include/exclude configuration.
5. For each medal, fetch activated-medal task progress and compute the remaining required interactions from server-provided text.
6. Perform the action within both the computed requirement and configured/custom limits.
7. Re-fetch progress after interaction rounds; stop when completed or free intimacy is full.
8. Continue other medals after individual failures; report an aggregate failure if any medal failed. Watch sessions run concurrently, at most **8** active sessions; likes/danmaku process medals sequentially.
9. The application-level action interceptors record a failed action but continue to the subsequent action.

Sources: [`Application/LiveFansMedalAppService.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Application/LiveFansMedalAppService.cs), [`LiveFansMedalTaskRunner.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/LiveFansMedalTaskRunner.cs), [`LiveFansMedalTaskPlanner.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/LiveFansMedalTaskPlanner.cs), [`TaskInterceptorAttribute.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Application/Attributes/TaskInterceptorAttribute.cs).

### Medal discovery, progress, and interactions

| Purpose | METHOD / full URL | Auth / endpoint headers | Parameters | Response and behavior |
|---|---|---|---|---|
| Bootstrap live Cookie | **GET** `https://api.live.bilibili.com/news/v1/notice/recom` | Account Cookie; browser UA. | Query `product=live`. | Require JSON `code=0`; merge response `Set-Cookie` into account Cookie. Used if `LIVE_BUVID` is absent. |
| Current medal list | **GET** `https://api.live.bilibili.com/xlive/app-ucenter/v1/fansMedal/panel` | Cookie; browser UA; live Referer/Origin. | Query `page=<1-based>`, `page_size=10`. | `$.data.list[]`, `special_list[]`; item `medal.target_id`, `medal.medal_id`, `medal.level`, `medal.medal_name`, `room_info.room_id`, `room_info.living_status`, `anchor_info.nick_name`; pagination `page_info.total_page`, `page_info.has_more`. |
| Current task progress | **GET** `https://api.live.bilibili.com/xlive/app-ucenter/v1/fansMedal/GetActivatedMedalInfo` | Cookie + CSRF query; browser UA; live Referer/Origin. | Query `target_id=<anchor UID>`, `csrf=<bili_jct>`, `web_location=444.260`. | `$.data.is_lighted`, `reach_free_intimacy_limit`, `level`, `task_info[]`: `title`, `sub_title`, `jump_type`, `is_done`. Action types: `like`, `sendDanmu`, `watchLive`. |
| Older medal-wall list, retained interface | **GET** `https://api.live.bilibili.com/xlive/web-ucenter/user/MedalWall` | Cookie; browser UA; live Referer/Origin. | Query `target_id=<account UID>`. | `$.data.list[]`: `live_status`, `target_name`, `link`, `medal_info.medal_name`, `medal_id`, `target_id`, `level`. **Not used by the current runner.** |
| Room status/area IDs | **GET** `https://api.live.bilibili.com/room/v1/Room/get_info` | Browser UA; method has no Cookie argument and no endpoint Referer/Origin declaration. | Query `room_id=<room ID>`, `from=room`. | `$.data.room_id`, `uid`, `area_id`, `parent_area_id`, `live_status`; runner only treats `live_status=1` as live. |
| Like room | **POST** `https://api.live.bilibili.com/xlive/app-ucenter/v1/like_info_v3/like/likeReportV3` | Cookie + CSRF; browser UA; live Referer/Origin. | URL-encoded `click_time=<batch size>`, `room_id`, `uid=<account UID>`, `anchor_id=<host UID>`, `csrf_token`, `csrf`. | `code=0` counts the requested likes. Send batches of **at most 10**. Only while room status is 1. Three consecutive failures end that medal’s action. |
| Send danmaku | **POST** `https://api.live.bilibili.com/msg/send` | Cookie + CSRF; browser UA; no endpoint Referer/Origin declaration. | URL-encoded `bubble=0`, `msg=<configured content>`, `color=16777215`, `mode=1`, `fontsize=25`, `rnd=<current Unix seconds>`, `roomid`, `csrf`, `csrf_token`. | `code=0` counts one message. Consecutive failure threshold configurable. |
| Wear medal, retained interface | **POST** `https://api.live.bilibili.com/xlive/app-ucenter/v1/fansMedal/wear` | Cookie + CSRF; browser UA; live Referer/Origin. | Form DTO exists. **Not called by the current runner.** | No current automatic wear/restore step. |

For current list/progress/status methods, nonzero code or missing data throws a business failure. Specific endpoint numeric eligibility/completion codes are **[UNCONFIRMED]**; completion is determined from task fields, not a special “already done” code.

Sources: [`ILiveApi.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Interfaces/ILiveApi.cs), [`FansMedalTasksResponse.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveApi/FansMedalTasksResponse.cs), [`MedalWallDto.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveApi/MedalWallDto.cs), [`GetLiveRoomInfoResponse.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveApi/GetLiveRoomInfoResponse.cs), [`LikeLiveRoomRequest.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveApi/LikeLiveRoomRequest.cs), [`SendLiveDanmukuRequest.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveApi/SendLiveDanmukuRequest.cs).

### Progress, intimacy, and skip rules

1. Enumerate up to 1,000 pages, pausing **500 ms** between pages. Stop only when `has_more=false` and current page is at least `total_page`. An incomplete traversal at the safety bound fails.
2. Require positive anchor UID and room ID. Merge `special_list` before `list`, deduplicate by anchor UID.
3. Select the first `task_info` whose `jump_type` matches the action.
4. Skip if no matching task, `is_done=true`, or **`is_lighted=true && reach_free_intimacy_limit=true`**.
5. Parse the first numeric quantity from `title`; valid range is `(0,1440]`. For watching multiply minutes by 60; round up to whole seconds/interactions.
6. Parse `current/limit` from `sub_title`; require `limit <= 1000`. Remaining work is `ceil(max(0, limit-current) × roundSize)`.
7. If progress text lacks `current/limit`, an **unlit** medal gets one lighting round; a lit medal with unparseable progress fails.
8. There is **no fixed intimacy-point cap encoded in this planner**. It follows the returned completion/free-intimacy flags and textual task requirements.
9. There is **no blanket skip-lit-medals option**. Lit medals continue through daily tasks unless completed/full. Levels above 20 are included; legacy `IsSkipLevel20Medal` is ignored.
10. Danmaku with `DanmakuOnlyWhenOffline=true` skips a currently live room. Independently, if a medal is unlit, the room is live, and likes are enabled, danmaku skips that room so likes can light it.
11. Interaction delays: **3 seconds after each like request**, **7 seconds after each danmaku request**, including failed attempts. These are additional to the common API delay.
12. After a round, refresh tasks. If an unlit medal becomes lit: custom-budget mode stops that medal’s action; daily-limit mode computes the new daily task and continues.
13. If a full round produced no progress, wait 3 seconds and refresh once; still no progress is a failure.

Sources: [`LiveFansMedalTaskPlanner.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/LiveFansMedalTaskPlanner.cs), [`LiveFansMedalTaskRunner.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/LiveFansMedalTaskRunner.cs), [`LiveFansMedalTaskTests.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/test/Ray.BiliBiliTool.DomainService.UnitTests/LiveFansMedalTaskTests.cs). Tests were read, not executed.

### Watch heartbeat `E` / `X`

Both endpoints use browser UA, account Cookie including `LIVE_BUVID`, CSRF, and **WBI signing in the current client**. Neither interface declares endpoint-specific Referer/Origin.

| Endpoint | URL-encoded body |
|---|---|
| **POST** `https://live-trace.bilibili.com/xlive/data-interface/v1/x25Kn/E` | `id=<JSON [parent_area_id,area_id,0,room_id]>`; `ruid=<anchor UID>`; `ts=<Unix milliseconds>`; `is_patch=0`; `heart_beat=[]` as a string; `ua=<browser UA>`; `csrf`, `csrf_token`; `visit_id=""`; `device=<JSON ["LIVE_BUVID","UUID"]>`; `web_location=444.8`; WBI `wts`, `w_rid`. |
| **POST** `https://live-trace.bilibili.com/xlive/data-interface/v1/x25Kn/X` | `s=<heartbeat HMAC signature>`; `id=<JSON [parent_area_id,area_id,sequence,room_id]>`; `ets=<previous response timestamp>`; `benchmark=<previous secret_key>`; `time=<previous heartbeat_interval>`; `ts=<current Unix milliseconds>`; `ua`; `csrf`, `csrf_token`; `visit_id=""`; same `device`; `ruid=<anchor UID>`; `trackid=-99998`; `web_location=444.8`; WBI `wts`, `w_rid`. |

Response for both: `$.data.heartbeat_interval`, `secret_key`, `secret_rule[]`, `timestamp`.

Numbered session flow:

1. Require room `live_status=1` and positive `parent_area_id`.
2. Generate one UUID for the watch session; keep `device=["LIVE_BUVID",UUID]` stable across E/X.
3. Send E with sequence 0. Entry itself does **not** count as watched time.
4. Require returned interval in **1–300 seconds** and nonempty secret key.
5. Wait the returned interval; re-query room status; stop if no longer live.
6. Send X beginning with sequence 1 using previous response secrets/timestamp.
7. Nonzero code or missing data increments consecutive failures; retry without advancing sequence/watched time. Stop at configured failure threshold.
8. On success, replace heartbeat state, increment sequence, and add the interval to watched seconds.
9. Refresh medal progress every ≥60 accumulated seconds or when the target is reached; stop early if the task is complete.
10. In daily-limit mode, if target time is reached but progress remains incomplete, wait 3 seconds and refresh; still incomplete is a failure.

Sources: [`ILiveTraceApi.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Interfaces/ILiveTraceApi.cs), [`EnterRoomRequest.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveTraceApi/EnterRoomRequest.cs), [`HeartBeatRequest.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveTraceApi/HeartBeatRequest.cs), [`HeartBeatResponse.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveTraceApi/HeartBeatResponse.cs), [`LiveFansMedalTaskRunner.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/LiveFansMedalTaskRunner.cs).

**Exact heartbeat-signature algorithm: embedded local implementation, no external signer.**

1. Serialize a compact JSON object in this property order:
   `platform="web"`, `parent_id`, `area_id`, `seq_id`, `room_id`, `buvid=<LIVE_BUVID>`, `uuid`, `ets`, `time=<heartbeat_interval>`, `ts=<Unix milliseconds>`.
2. Initialize `result` to that JSON string.
3. Iterate `secret_rule` in response order. For each rule compute HMAC over **UTF-8 bytes of the current result**, with **UTF-8 bytes of `secret_key`** as the key.
4. Rule mapping: `0→HMAC-MD5`, `1→HMAC-SHA1`, `2→HMAC-SHA256`, `3→HMAC-SHA224`, `4→HMAC-SHA512`, `5→HMAC-SHA384`.
5. Convert each HMAC result to **lowercase hexadecimal text**. The next round hashes that hex text, not the raw digest bytes.
6. Final text is `s`. An empty rule list leaves the initial JSON unchanged; an unknown rule throws.

**Additional WBI signature:** E/X DTOs contain `wts`, causing the HTTP handler to sign their forms. Fetch WBI image/sub keys from nav; concatenate keys and permute using the standard WBI index table, taking the first 32 selected characters. Remove old `wts/w_rid`; add current Unix-seconds `wts`; strip `[!'()*]` from parameter values; percent-encode keys/values; sort keys; join as `k=v&...`; append mixin key; lowercase MD5 becomes `w_rid`. Whether E/X require WBI on today’s server is **[UNCONFIRMED]**; this is the observed client behavior.

Sources: [`LiveHeartBeatCrypto.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Utils/LiveHeartBeatCrypto.cs), [`HeartBeatRequest.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveTraceApi/HeartBeatRequest.cs), [`WridEncryptionDelegatingHandler.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/WridEncryptionDelegatingHandler.cs), [`WbiService.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Services/WbiService.cs).

### Medal configuration

All keys under `LiveFansMedalTaskConfig`.

| Key | Default | Meaning / effective bound |
|---|---|---|
| `IsEnable` | `true` | Enable task. |
| `Cron` | `0 5 0 * * ?` | Daily 00:05. |
| `FollowDailyTaskLimit` | `false` | False uses custom budgets, still bounded by remaining server task. True follows remaining daily requirements. |
| `EnableLike`, `EnableDanmaku`, `EnableWatch` | all `true` | Per-action switches. |
| `DanmakuOnlyWhenOffline` | `false` | Restrict danmaku to rooms not currently live. |
| `OnlySelectedAnchors` | `false` | Restrict to included UIDs. |
| `IncludedAnchorIds` | `""` | Comma-separated positive anchor UIDs; used when selection-only enabled. |
| `ExcludedAnchorIds` | `""` | Comma-separated positive UIDs; exclusion wins. |
| `DanmakuContent` | `OvO` | Required, maximum 30 characters by option validation. |
| `LikeNumber` | `30` | Custom per-medal budget, clamp 0–5000; **5000 total likes per action/account runner** additionally enforced. |
| `SendDanmakuNumber` | `1` | Custom per-medal budget, clamp 0–100. |
| `HeartBeatNumber` | `70` | **Minutes**, despite its name; custom per-medal watch target, clamp 0–1440. |
| `HeartBeatSendGiveUpThreshold` | `5` | Consecutive X failures, clamp 1–10. |
| `SendDanmakugiveUpThreshold` | `3` | Consecutive danmaku failures, clamp 1–10; preserve spelling. |

Daily-limit mode safety bounds are 5000 likes, 100 danmaku, or 86,400 watch seconds per medal, further limited by remaining tasks. A zero custom budget suppresses an action only in custom-budget mode. `HeartBeatInterval=60` exists as a constant, but the current runner uses the server-returned interval.

Sources: [`LiveFansMedalTaskOptions.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Config/Options/LiveFansMedalTaskOptions.cs), [`LiveFansMedalTaskRunner.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/LiveFansMedalTaskRunner.cs), [`Web/appsettings.json`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Web/appsettings.json).

## 3. Relations and `UnfollowBatched`

### Endpoint specification

All endpoints use Cookie `SESSDATA` and browser UA; POSTs use `csrf=<bili_jct>`. None uses WBI or app signing in this client.

| METHOD / full URL | Headers beyond shared defaults | Query/body | Response / known codes |
|---|---|---|---|
| **GET** `https://api.bilibili.com/x/relation/tags` | `Referer: https://space.bilibili.com/<account UID>/fans/follow`; `Sec-Fetch-Mode: no-cors`; `Sec-Fetch-Dest: script`. | Query `jsonp=jsonp`. | `$.data[]`: `tagid`, `name`, `count`, optional `tip`. Historical documentation: `0` success, `-101` not logged in; tag `-10` special follows, `0` default group. |
| **GET** `https://api.bilibili.com/x/relation/tag` | `Referer: https://space.bilibili.com/`; `Cache-Control: no-cache`; `Pragma: no-cache`. | Query `mid=<account UID>`, `tagid`, `pn=1-based`, `ps=20`, `jsonp=jsonp`. | `$.data[]`: creator `mid`, `uname`; historical docs also describe `mtime`, relationship fields. Historical codes: `0`, `-101`, `-400`, `22104` group nonexistent. |
| **GET** `https://api.bilibili.com/x/relation/followings` | `Referer: https://space.bilibili.com/`. | Query `vmid=<account UID>`, `order_type=""` for follow-time order or `attention` for visit-frequency order; `order=desc`, `pn=1`, `ps=20`, `jsonp=jsonp`. | `$.data.list[]`: `mid`, `uname`; `$.data.total`. Historical codes `0`, `-400`, `22007` access beyond first five pages when viewing another user. Historical default `ps=50` differs from client’s explicit `20`. |
| **POST** `https://api.bilibili.com/x/relation/tag/create?cross_domain=true` | `Origin: https://space.bilibili.com`; Referer defaults in interface to `https://space.bilibili.com/{0}/fans/follow`; lottery call does not supply the formatted override. | URL-encoded `tag=<group name>`, `csrf`. | `$.data.tagid`. Historical codes: `0`, `-101`, `-111` CSRF failure, `-400`, `22106` group exists, `22103` name too long; historical maximum name length 16 characters. |
| **POST** `https://api.bilibili.com/x/relation/tags/copyUsers` | `Origin: https://space.bilibili.com`; `Referer: https://space.bilibili.com/<account UID>/fans/follow?tagid=-1`. | URL-encoded `fids=<comma-separated creator UIDs>`, `tagids=<comma-separated group IDs>`, `csrf`, `jsonp=jsonp`. | Client uses `code/message` only. Historical codes `0`, `-101`, `-111`, `-400`, `22104` no permission, `22105` not following. |
| **POST** `https://api.bilibili.com/x/relation/modify` | `Origin: https://space.bilibili.com`; `Referer: https://space.bilibili.com/<account UID>/fans/follow?tagid=<selected tag ID>`. | URL-encoded `fid=<creator UID>`, `act=2`, `re_src=11`, `csrf`, `jsonp=jsonp`. | Client uses `code/message`; `0` counts success. Historical codes `-101` not logged in, `-102` account banned, `-111` CSRF failure, `-400` request error, `22003` user in blacklist. |
| Historical movement endpoint, **not used by current grouping** | **POST** `https://api.bilibili.com/x/relation/tags/moveUsers`; browser-space Origin/Referer requirements **[UNCONFIRMED]** from historical reference. | URL-encoded `beforeTagids=<old groups>`, `afterTagids=<new groups>`, `fids=<UIDs>`, `csrf`; Cookie authentication. | Historical codes same as copy: `0`, `-101`, `-111`, `-400`, `22104`, `22105`. |

Historical `act` values: **1 follow; 2 unfollow; 3 private follow; 4 cancel private follow; 5 blacklist; 6 remove blacklist; 7 remove follower**. Historical `re_src`: **11 space; 14 video; 115 article; 222 activity page**. Only `act=2/re_src=11` is exercised by the current batch-unfollow implementation; current support for the other values is **[UNCONFIRMED]**.

Sources: [`IApiApi.cs`, relation region](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Interfaces/IApiApi.cs), [`Agent/Constants.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/Constants.cs), [`GetSpecialFollowingsRequest.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/Relation/GetSpecialFollowingsRequest.cs), [`GetFollowingsRequest.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/Relation/GetFollowingsRequest.cs), [`FollowingsOrderType.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/Relation/FollowingsOrderType.cs), [`ModifyRelationRequest.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/Relation/ModifyRelationRequest.cs), [`CopyUserToGroupRequest.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/Relation/CopyUserToGroupRequest.cs), [preserved historical `user/relation.md`](https://github.com/RayWangQvQ/bilibili-API-collect/blob/93092f26348d33468fd4799ca4c84b9522c8ccab/user/relation.md).

### Numbered unfollow flow

1. Check task enabled; ensure cookies.
2. Query tags; select exact configured `GroupName`.
3. Missing group or zero `count`: log and return.
4. Let desired count be configured `Count`; `-1` becomes current group total.
5. Calculate last page as `ceil(total/20)`.
6. Fetch last page, reverse its returned array, then fetch earlier pages as needed, reversing each array.
7. Build target candidates before issuing mutations; process no more than the desired count.
8. For each candidate, skip `RetainUids`; otherwise POST `modify` with `act=2`, `re_src=11`.
9. Nonzero mutation result logs its message and continues; count only successful mutations.
10. Re-query tags and log remaining members.

**Count semantics:** whitelist entries and failed unfollows still consume candidate positions; the task does not fetch replacements to reach the requested number of successful removals. It reverses server order rather than sorting `mtime` locally.

**Delays:** no extra explicit per-unfollow sleep in `AccountDomainService`; all list/mutation calls use common API spacing. No dedicated account-to-account sleep appears in the sequential multi-account runner.

Configuration under `UnfollowBatchedTaskConfig`: `IsEnable=true`; `Cron="0 0 6 1 * ?"`; `GroupName="天选时刻"`; `Count=20`; `RetainUids=""`. Whitelist is comma-separated, trimmed, empty entries removed.

Sources: [`Application/UnfollowBatchedTaskAppService.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Application/UnfollowBatchedTaskAppService.cs), [`AccountDomainService.cs`, `UnfollowBatched`, `GetTag`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/AccountDomainService.cs), [`UnfollowBatchedTaskOptions.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Config/Options/UnfollowBatchedTaskOptions.cs), [`BaseMultiAccountsAppService.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Application/BaseMultiAccountsAppService.cs).

## 4. Notifications

### Numbered aggregation and content flow

1. Each configured Serilog destination receives events admitted by both the global minimum and its `restrictedToMinimumLevel`.
2. Default push-sink minimum in Console/Web JSON is **Information**. Warning/Error/Fatal are included; Debug API traces are excluded unless changed.
3. Batched sinks queue by Serilog property **`GroupKey`**; absent/empty property becomes `"Unknown"`.
4. Default formatting is **`{Message:lj}{NewLine}{Exception}`**: rendered log message plus exception, without timestamp/level prefix. Messages contain task start/end delimiters, account separators, usernames/status, action outcomes, errors, and final version/source attribution.
5. Default `sendBatchesAsOneMessages=true`; `batchSizeLimit=int.MaxValue`; formatted events are concatenated with trailing blank lines.
6. Web jobs set `GroupKey=<Quartz FireInstanceId>` and flush that group at job completion. This combines accounts belonging to that job run.
7. Console closes/flushed Serilog on process exit; ungrouped run logs are flushed by sink disposal.
8. Docs describe `Notification:IsSingleAccountSingleNotify=true` as a **legacy** setting absent from current JSON. Current multi-account execution does not perform per-account sink flushes; do not infer operational per-account delivery from that documented legacy key.
9. The Web base job calls Flush without a title. Channels with title fields use their fallback titles below. The base `PushService` calls `BuildMsg(message)` without forwarding its title argument.
10. Sink transport code logs HTTP status/response to Serilog SelfLog; it does not validate each service’s JSON business-success code.

Sources: [`Web/Jobs/BaseJob.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Web/Jobs/BaseJob.cs), [`Console/Program.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Console/Program.cs), [`BaseMultiAccountsAppService.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Application/BaseMultiAccountsAppService.cs), [`docs/configuration.md`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/docs/configuration.md), dependency [`BatchedSink.cs`](https://github.com/RayWangQvQ/Ray.Serilog.Sinks/blob/0acd4cd0ac50bcfea5b0ad2ac874ca15301b13c0/src/Ray.Serilog.Sinks.Batched/BatchedSink.cs), [`PushService.cs`](https://github.com/RayWangQvQ/Ray.Serilog.Sinks/blob/0acd4cd0ac50bcfea5b0ad2ac874ca15301b13c0/src/Ray.Serilog.Sinks.Batched/PushService.cs).

### HTTP transport matrix

Notation: `M` is the formatted batch described above. JSON requests use UTF-8 `application/json`; ServerChan uses URL-encoded forms. Bilibili cookies/CSRF are not used for push services.

| Channel / JSON sink index | METHOD / URL / credential placement | Actual body and formatting | Source |
|---|---|---|---|
| Telegram / **3** | **POST** `https://api.telegram.org/bot{botToken}/sendMessage`; token in URL path; optional HTTP proxy. | `{"chat_id":"<chatId>","text":"<b></b>\n\n<M>","parse_mode":"HTML","disable_web_page_preview":true}` using host newline conventions. Title is empty in normal flushes. The implementation does not HTML-escape the log batch. Sink constructs client with 5-second timeout. | [`TelegramApiClient.cs`](https://github.com/RayWangQvQ/Ray.Serilog.Sinks/blob/0acd4cd0ac50bcfea5b0ad2ac874ca15301b13c0/src/Ray.Serilog.Sinks.TelegramBatched/TelegramApiClient.cs), [`TelegramBatchedSink.cs`](https://github.com/RayWangQvQ/Ray.Serilog.Sinks/blob/0acd4cd0ac50bcfea5b0ad2ac874ca15301b13c0/src/Ray.Serilog.Sinks.TelegramBatched/TelegramBatchedSink.cs). |
| PushPlus / **9** | **POST** **`http://www.pushplus.plus/send`**; token in JSON. | `{"token":"…","topic":"…","channel":"wechat","webhook":"…","title":"PushPlus Notification","content":"<M with newlines replaced by <br/>>","template":"html"}`. Configured valid channel overrides `wechat`; blank/unrecognized falls back to `wechat`. `webhook` is a service code, not URL. | [`PushPlusApiClient.cs`](https://github.com/RayWangQvQ/Ray.Serilog.Sinks/blob/0acd4cd0ac50bcfea5b0ad2ac874ca15301b13c0/src/Ray.Serilog.Sinks.PushPlusBatched/PushPlusApiClient.cs), sibling `Constants.cs`. |
| WeCom group bot / **4** | **POST** configured complete `webHookUrl`; usually `https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=<key>`. Client does not construct or validate that URL template. | Default **text**, not markdown: `{"msgtype":"text","text":{"content":"## Work Weixin Notification \r\n\r\n<M>"}}`. Optional `msgType=markdown` changes body to `{"msgtype":"markdown","markdown":{"content":"…"}}`. Newlines normalized to CRLF. | [`WorkWeiXinApiClient.cs`](https://github.com/RayWangQvQ/Ray.Serilog.Sinks/blob/0acd4cd0ac50bcfea5b0ad2ac874ca15301b13c0/src/Ray.Serilog.Sinks.WorkWeiXinBatched/WorkWeiXinApiClient.cs), [`WorkWeiXinLoggerConfigurationExtensions.cs`](https://github.com/RayWangQvQ/Ray.Serilog.Sinks/blob/0acd4cd0ac50bcfea5b0ad2ac874ca15301b13c0/src/Ray.Serilog.Sinks.WorkWeiXinBatched/WorkWeiXinLoggerConfigurationExtensions.cs). |
| DingTalk / **5** | **POST** configured complete webhook, conventionally `https://oapi.dingtalk.com/robot/send?access_token=<token>`. Optional `secret=` handling described below. | `{"msgtype":"markdown","markdown":{"title":"DingDing Notification","text":"<M with doubled newlines>"}}`. | [`DingTalkApiClient.cs`](https://github.com/RayWangQvQ/Ray.Serilog.Sinks/blob/0acd4cd0ac50bcfea5b0ad2ac874ca15301b13c0/src/Ray.Serilog.Sinks.DingTalkBatched/DingTalkApiClient.cs), sibling `Constants.cs`. |
| ServerChan Turbo / **6** | **POST** `https://sctapi.ftqq.com/{turboScKey}.send`; key in path. Preferred when `turboScKey` nonempty. | Form `title=ServerChan Notification&desp=<M with doubled newlines>`. | [`ServerChanTurboApiClient.cs`](https://github.com/RayWangQvQ/Ray.Serilog.Sinks/blob/0acd4cd0ac50bcfea5b0ad2ac874ca15301b13c0/src/Ray.Serilog.Sinks.ServerChanBatched/ServerChanTurboApiClient.cs). |
| Legacy ServerChan / **6** | **POST** `http://sc.ftqq.com/{scKey}.send`; used if Turbo key absent. | Form `text=ServerChan Notification&desp=<M with doubled newlines + legacy-service deprecation notice>`. | [`ServerChanApiClient.cs`](https://github.com/RayWangQvQ/Ray.Serilog.Sinks/blob/0acd4cd0ac50bcfea5b0ad2ac874ca15301b13c0/src/Ray.Serilog.Sinks.ServerChanBatched/ServerChanApiClient.cs). |
| Teams / **10** | **POST** configured complete `webhook`; URL itself holds credential. | **`{"text":"<M with newlines replaced by <br/>>"}`**. No adaptive card, MessageCard wrapper, or separate title is sent. | [`MicrosoftTeamsApiClient.cs`](https://github.com/RayWangQvQ/Ray.Serilog.Sinks/blob/0acd4cd0ac50bcfea5b0ad2ac874ca15301b13c0/src/Ray.Serilog.Sinks.MicrosoftTeamsBatched/MicrosoftTeamsApiClient.cs). |
| CoolPush / **7** | **POST** `https://push.xuthus.cc/send/{sKey}`; key in path. | **Raw text** body containing empty title/newline plus batch, with doubled newlines, despite content type `application/json`. It does **not** serialize a JSON object or JSON string. | [`CoolPushApiClient.cs`](https://github.com/RayWangQvQ/Ray.Serilog.Sinks/blob/0acd4cd0ac50bcfea5b0ad2ac874ca15301b13c0/src/Ray.Serilog.Sinks.CoolPushBatched/CoolPushApiClient.cs). |
| Custom API / **8** | **POST** configured `api`; no separate token/header option in this client. Credentials can be embedded in configured URL/body. | Replace every configured placeholder, default `#msg#`, with **JSON serialization of M**, then send the resulting string. Default template: `{"msgtype":"markdown","markdown":{"content":#msg#}}`. Placeholder must not have another pair of surrounding quotes. | [`OtherApiClient.cs`](https://github.com/RayWangQvQ/Ray.Serilog.Sinks/blob/0acd4cd0ac50bcfea5b0ad2ac874ca15301b13c0/src/Ray.Serilog.Sinks.OtherApiBatched/OtherApiClient.cs). |
| WeCom application / **11**, also supported | **GET** `https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=<corpId>&corpsecret=<secret>`; then **POST** `https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=<returned token>`. | Token response `errcode=0`, `access_token`. Message JSON `{"touser":"@all","toparty":"","totag":"","agentid":"<agentId>","msgtype":"text","text":{"content":"<M>"}}`. Obtains token for each push; exceptions yield an empty token. | [`WorkWeiXinAppApiClient.cs`](https://github.com/RayWangQvQ/Ray.Serilog.Sinks/blob/0acd4cd0ac50bcfea5b0ad2ac874ca15301b13c0/src/Ray.Serilog.Sinks.WorkWeiXinAppBatched/WorkWeiXinAppApiClient.cs). |
| Gotify / **12**, also supported | **POST** `{host}/message`; token attached as `X-Gotify-Key` to the content headers in this implementation. | `{"title":"Gotify Notification","message":"<M>","extras":{"extras":{"client::display":{"contentType":"text/markdown"}}}}`. The nested `extras.extras` shape is what the dependency constructs. | [`GotifyApiClient.cs`](https://github.com/RayWangQvQ/Ray.Serilog.Sinks/blob/0acd4cd0ac50bcfea5b0ad2ac874ca15301b13c0/src/Ray.Serilog.Sinks.GotifyBatched/GotifyApiClient.cs). |

**Confirmed documentation/dependency discrepancies:**

- Configuration docs say DingTalk signature security is unsupported. Dependency `0.1.6` **does implement** detection of `secret=` in the configured URL: compute Unix-ms timestamp, sign UTF-8 `timestamp+"\n"+secret` using HMAC-SHA256 with UTF-8 secret key, Base64 the digest, append `&timestamp=...&sign=...`. It retains the original `secret=` URL parameter and does not explicitly percent-encode Base64. Current server acceptance of that construction is **[UNCONFIRMED]**.
- Console/Web JSON includes Telegram `apiHost`, but the inspected `0.1.6` sink extension/client has **no `apiHost` argument** and hard-codes `https://api.telegram.org/bot`. Reverse-proxy configuration through that key is therefore **[UNCONFIRMED]**, with a concrete signature mismatch in source.
- The default WeCom bot sink message type is `text`, although the custom-API sample uses markdown.

Sources: [`docs/configuration.md`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/docs/configuration.md), [`TelegramLoggerConfigurationExtensions.cs`](https://github.com/RayWangQvQ/Ray.Serilog.Sinks/blob/0acd4cd0ac50bcfea5b0ad2ac874ca15301b13c0/src/Ray.Serilog.Sinks.TelegramBatched/TelegramLoggerConfigurationExtensions.cs), channel clients above.

### Explicit reminders

- Expired Cookie reminder: Warning event with title **`BiliBili Cookie 已过期`**; body identifies account as `***<last up to four UID digits>`, says the activity was skipped, instructs reopening account management and scanning login, and states a once-per-account-per-day reminder limit.
- Scheduled failure summary: Warning event titled **`BiliBili 任务失败汇总`**, body lists task name, failure count, masked accounts, and instructions to review execution records/retry.
- Failure merging waits until no tasks are active and both last activity/last failure are at least **2 minutes** old; worker checks every **10 seconds**. Failed delivery attempts are spaced by **15 minutes**.
- Adapter turns notifications into a log line: **`[NOTIFICATION] <Title>: <Body>`**. Its `groupKey` argument is not applied to Serilog grouping or used to force a flush.
- Docs expose `CookieCheck:NotifyEnabled`; the inspected `CookieTaskGuard` checks `AutoCheckEnabled` but does not itself consult `NotifyEnabled` before constructing its reminder. Do not assume that documented switch gates this code path without further implementation changes.

Sources: [`CookieTaskGuard.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Application/CookieTaskGuard.cs), [`SerilogNotificationAdapter.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Infrastructure/Notifications/SerilogNotificationAdapter.cs), [`TaskFailureBatchMonitor.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Web/Services/TaskFailureBatchMonitor.cs), [`TaskFailureNotificationWorker.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Web/Services/TaskFailureNotificationWorker.cs).

## 5. Overall configuration and schedules

### Loading and precedence

1. Console: `appsettings.json` → `appsettings.{Environment}.json` → development user secrets → `Ray_`-prefixed environment → unprefixed environment → command-line arguments → optional `cookies.json`.
2. Use `__` for nested environment keys; e.g. `Ray_LiveLotteryTaskConfig__AutoGroupFollowings=false`.
3. Console `RunTasks` selects tasks separated by `&`; default empty prompts interactively. Web uses Quartz rather than `RunTasks`.
4. Web adds SQLite-backed configuration after its initial configuration providers; the panel persists settings there.
5. All task sections derived from `BaseConfigOptions` have `IsEnable=true` and a `Cron` property.

Sources: [`docs/configuration.md`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/docs/configuration.md), [`Console/Program.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Console/Program.cs), [`Web/Program.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Web/Program.cs), [`BaseConfigOptions.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Config/Options/BaseConfigOptions.cs).

### Security settings — distinguish docs, JSON, and class defaults

| Key under `Security` | Documented default | Console JSON | Web effective class fallback where omitted | Meaning |
|---|---|---|---|---|
| `IsSkipDailyTask` | `false` | `false` | `false` | Console precheck stops task execution. |
| `RandomSleepMaxMin` | `0` Console | `0` | Class `10`; Web job path does not call Console random-sleep routine. | Console initial random delay; 0 disables; Login/Test skip it. Implementation selects 1–N minutes when enabled. |
| `IntervalSecondsBetweenRequestApi` | **20** | **10** | **3** | Pre-request delay control; docs disagree with active defaults. |
| `IntervalMethodTypes` | `GET,POST` | `GET,POST` | `GET,POST` | Comma-separated methods subject to delay. |
| `UserAgent` | Windows desktop Edge/Chrome 124 | Same | Explicit same value | Browser UA for web/live/relation/trace calls. |
| `UserAgentApp` | Android 12 SM-S9080, BiliApp 7.76.0/build 7760700 | Same | Explicit same value | App requests elsewhere. |
| `WebProxy` | Empty | Empty | Empty | Optional proxy; plain `http://host:port` or authenticated `user:password@host:port`. |
| Account interval | No documented key found | None | None in multi-account loop | Accounts execute sequentially without dedicated account sleep. |

Exact browser default:

```text
Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0
```

**Actual API spacing:** when interval >0 and request method is selected, sleep before sending. Ordinary delay is a random integer in **`floor(configuredSeconds/2)..configuredSeconds` inclusive**; Join is fixed 3 seconds; E/X fixed 1 second. It is **not** an elapsed-time limiter guaranteeing a minimum gap between all requests, especially concurrent watches.

Sources: [`SecurityOptions.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Config/Options/SecurityOptions.cs), [`IntervalDelegatingHandler.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/HttpClientDelegatingHandlers/IntervalDelegatingHandler.cs), [`BiliBiliToolHostedService.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Console/BiliBiliToolHostedService.cs), [`Console/appsettings.json`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Console/appsettings.json), [`Web/appsettings.json`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Web/appsettings.json).

### Configuration section inventory

The guide’s reference headings are Cookies, Security, Daily tasks, Live lottery, Batch unfollowing, VIP big points, B-coin coupon charging, Notifications, Logging/scheduling, and Automatic recovery. The concrete sections/keys they document, plus current task sections in appsettings, are:

| Section | Defaults / surface / meaning |
|---|---|
| `BiliBiliCookies` | Numbered Cookie strings `1`, `2`, …; default empty. |
| `Security` | UA, app UA, proxy, Console skip/random sleep, API delay/method selection; table above. |
| `DailyTaskConfig` | `IsWatchVideo=true`, `IsShareVideo=true`, `NumberOfCoins=5` capped 0–5, `NumberOfProtectedCoins=0`, `SelectLike=true`, `SupportUpIds=""`, `SaveCoinsWhenLv6=false`, `IsDonateCoinForArticle=false`, `DevicePlatform=android`; enable/cron. |
| Legacy daily scheduling keys | `DayOfReceiveVipPrivilege` documented default 1, scheduling check now commented out; `DayOfExchangeSilver2Coin` documented default -1. Separate job crons supersede these. |
| `LiveLotteryTaskConfig` | Complete keys/defaults in §1. |
| `LiveFansMedalTaskConfig` | Complete current options in §2; guide mentions its cron but does not provide the full modern medal option surface. |
| `UnfollowBatchedTaskConfig` | Complete keys/defaults in §3. |
| `VipBigPointConfig` | `ViewBangumis="33378"` season ID; enable/cron. |
| `ChargeTaskConfig` | `AutoChargeUpId="-1"` Console, `""` Web; `ChargeComment=""`; enable/cron. Docs warn self-charging is no longer allowed and unspecified recipient uses fallback. |
| `MangaTaskConfig` | `CustomComicId=27355`, `CustomEpId=381662`; enable/cron. |
| `MangaPrivilegeTaskConfig` | Enable/cron. |
| `Silver2CoinTaskConfig` | Enable/cron. |
| `VipPrivilegeConfig` | Enable/cron. |
| `WatchLaterCleanupTaskConfig` | `OnlyViewed=true`, `PageSize=50`, `DeleteInvalid=true`, `DeleteIntervalMs=1000`, `BackupBeforeClean=true`, `BackupDirectory="backup"`, `BackupRetentionDays=7`, `ViewedThreshold=0.95`, `UseHistoryCrossCheck=true`, `HistoryMaxPages=10`; enable/cron. Current appsettings section beyond guide’s detailed reference. |
| `Notification` | Legacy `IsSingleAccountSingleNotify=true` documented; absent from active JSON and not reflected by current account flush behavior. |
| `CookieCheck` | `AutoCheckEnabled=true`, `NotifyEnabled=true`, `StateFile="config/cookie-check-state.json"` documented. |
| `TaskFailureNotification` | `Enabled=true`, `Tasks:<AppServiceName>=true`, `StateFile="config/task-failure-batch.json"`. |
| `Serilog` | Console, debug, file, and push destinations. Global minimum Console **Debug**, Web **Information**; console/push destinations normally Information. Console template `[{Timestamp:HH:mm:ss} {Level:u3}] {Message:lj}{NewLine}{Exception}`. File `Logs/log.txt`, rolling interval enum value 3. |
| `AutoRecoverConfig` | `IsEnable=true`, `IntervalHours=2` clamped 1–24, `RecordRetentionDays=3` documented range 1–90; no cron. |

**Push configuration keys:** `Serilog:WriteTo:<index>:Args:<key>`; all credentials default empty.

| Index | Name | Args beyond common `restrictedToMinimumLevel=Information` |
|---|---|---|
| 3 | `TelegramBatched` | `botToken`, `chatId`, `proxy`; JSON also has `apiHost=https://api.telegram.org`, discrepancy above. |
| 4 | `WorkWeiXinBatched` | `webHookUrl`. |
| 5 | `DingTalkBatched` | `webHookUrl`. |
| 6 | `ServerChanBatched` | `scKey`, `turboScKey`. |
| 7 | `CoolPushBatched` | `sKey`. |
| 8 | `OtherApiBatched` | `api`, `placeholder=#msg#`, `bodyJsonTemplate={"msgtype":"markdown","markdown":{"content":#msg#}}`. |
| 9 | `PushPlusBatched` | `token`, `channel`, `topic`, `webhook`; all empty, effective channel fallback `wechat`. |
| 10 | `MicrosoftTeamsBatched` | `webhook`. |
| 11 | `WorkWeiXinAppBatched` | `corpId`, `agentId`, `secret`, `toUser=@all`, `toParty=""`, `toTag=""`. |
| 12 | `GotifyBatched` | `host`, `token`. |

Additional host/configuration sections present in JSON: Console `RunTasks=""`, `PlatformType="Unknown"`, `IsPrd=false`, `QingLongConfig:{ClientId:"",ClientSecret:""}`, optional `DeviceCookie:{Buvid3:"",Buvid4:"",BNut:""}`; Web `PlatformType="Web"`, `AllowedHosts="*"`, `ConnectionStrings:Sqlite="Data Source=./config/BiliBiliTool.db;Cache=Shared"`.

**Production override discrepancy:** guide says Console Production overrides charging recipient, but inspected `appsettings.Production.json` places `AutoChargeUpId="341688380"` under **`DailyTaskConfig`**, not `ChargeTaskConfig`. Do not treat that as a confirmed effective override for `ChargeTaskConfig:AutoChargeUpId`.

Sources: [`docs/configuration.md`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/docs/configuration.md), [`Console/appsettings.json`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Console/appsettings.json), [`Console/appsettings.Production.json`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Console/appsettings.Production.json), [`Web/appsettings.json`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Web/appsettings.json).

### Default Web/Docker schedules

Docker runs the Web host and uses these Quartz expressions, including seconds. No explicit timezone is set in the inspected `WithCronSchedule` calls; do not assume Asia/Shanghai.

| Task / configuration section | Default Quartz cron | Intended wall-clock occurrence |
|---|---|---|
| Daily / `DailyTaskConfig` | `0 0 15 * * ?` | Daily 15:00 |
| Manga / `MangaTaskConfig` | `0 0 14 * * ?` | Daily 14:00 |
| Manga privileges / `MangaPrivilegeTaskConfig` | `0 0 15 * * ?` | Daily 15:00 |
| Silver→coin / `Silver2CoinTaskConfig` | `0 0 8 * * ?` | Daily 08:00 |
| Charge / `ChargeTaskConfig` | `0 0 12 28 * ?` | Monthly day 28, 12:00 |
| VIP privileges / `VipPrivilegeConfig` | `0 0 1 * * ?` | Daily 01:00 |
| VIP big points / `VipBigPointConfig` | `0 7 1 * * ?` | Daily 01:07 |
| Live lottery / `LiveLotteryTaskConfig` | `0 0 22 * * ?` | Daily 22:00 |
| Live medals / `LiveFansMedalTaskConfig` | `0 5 0 * * ?` | Daily 00:05 |
| Batch unfollow / `UnfollowBatchedTaskConfig` | `0 0 6 1 * ?` | Monthly day 1, 06:00 |
| Watch-later cleanup / `WatchLaterCleanupTaskConfig` | `0 30 3 * * ?` | Daily 03:30 |
| Login and Test jobs; missing-cron fallback | `0 0 0 1 1 ?` | Annual January 1, 00:00; code comments call this disabled-by-default, but it is a real annual cron. |
| Auto recovery | Simple trigger, no cron | First trigger one minute after registration, then every `IntervalHours` hours; default 2. |

All listed task JSON switches default true. Startup resumes/pauses their persistent cron triggers based on `IsEnable`.

**Qinglong differs:** launcher comments use five-field cron: lottery **`0 13 * * *`**, medals **`5 0 * * *`**, unfollow **`0 12 1 * *`**. Qinglong calls Console with `Ray_RunTasks`, rather than using the Web cron settings.

Sources: [`ServiceCollectionQuartzConfiguratorExtensions.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Web/Extensions/ServiceCollectionQuartzConfiguratorExtensions.cs), [`SchedulerConfigurationStartup.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Web/Services/SchedulerConfigurationStartup.cs), [`Web/appsettings.json`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Web/appsettings.json), [`bili_task_liveLottery.sh`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/platforms/qinglong/DefaultTasks/bili_task_liveLottery.sh), [`bili_task_liveFansMedal.sh`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/platforms/qinglong/DefaultTasks/bili_task_liveFansMedal.sh), [`bili_task_unfollowBatched.sh`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/platforms/qinglong/DefaultTasks/bili_task_unfollowBatched.sh).

## 6. Host architecture — 8 lines

1. C#/.NET 10 solution with separate Console and ASP.NET Core Web entry points.
2. Console uses Generic Host plus an `IHostedService`, selects task app services, executes them, then stops.
3. Both hosts share Application orchestration → DomainService behavior → Refit HTTP API interfaces/DTOs.
4. Multi-account app services process configured accounts sequentially and write per-account execution records.
5. Web serves Blazor interactive server/WebAssembly components, MudBlazor UI, controllers, and Swagger.
6. Quartz jobs invoke shared task services; BlazingQuartz supplies scheduling UI; Quartz persists `QRTZ_` tables in SQLite.
7. SQLite also stores panel configuration, EF-managed application data, and Serilog logs; reminder state additionally uses JSON files.
8. Docker launches the Web DLL; Qinglong shell scripts launch Console from source or a downloaded executable with environment-selected task codes.

Sources: [`Console/Program.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Console/Program.cs), [`BiliBiliToolHostedService.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Console/BiliBiliToolHostedService.cs), [`Web/Program.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Web/Program.cs), [`ServiceCollectionQuartzConfiguratorExtensions.cs`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Web/Extensions/ServiceCollectionQuartzConfiguratorExtensions.cs), [`platforms/docker/entrypoint.sh`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/platforms/docker/entrypoint.sh), [`platforms/qinglong/DefaultTasks/bili_task_base.sh`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/platforms/qinglong/DefaultTasks/bili_task_base.sh).