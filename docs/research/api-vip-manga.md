# VIP / manga / charging protocol specification

**Evidence:** source inspection of BiliBiliToolPro `main`, initially resolved to commit `4ab02ce28cae854a3d84c713a3ccb483006b8213`; no authenticated Bilibili requests were executed. Protocol facts only; no implementation code copied.

**API-collect availability:** the requested [SocialSisterYi repository](https://github.com/SocialSisterYi/bilibili-API-collect) redirects to a repository whose documentation was deleted. API-collect facts below were recovered from its [published documentation mirror](https://goooler.github.io/bilibili-API-collect/) and [preserved source fork](https://github.com/bilibili-plugins/bilibili-api-collect). These are explicitly distinguished from BiliBiliToolPro behavior.

**bili-daily implementation update:** receipt now carries the existing `vipPointParams` metadata, including `build`; [upstream issue #1195](https://github.com/RayWangQvQ/BiliBiliToolPro/issues/1195) reports missing build causing `6007000`. This is a community observation, not an official compatibility guarantee. OGV receipt is tracked separately from playback: successful receipt is followed by a manual-watch instruction, never a generic completion request or a claim of 40 points. The captured signing recipe below is historical; newer reports differ, and it is not implemented here. Privilege and voucher-charge tasks gate on active annual membership; monthly VIP scoring and manga benefits remain available.

## Shared authentication, headers, and eligibility

1. Every target task first checks its section’s `IsEnable`, sets account cookies, and calls **GET `https://api.bilibili.com/x/web-interface/nav`**.
   - Auth: Cookie containing `SESSDATA`; BiliBiliToolPro additionally validates `DedeUserID` and `bili_jct` in its cookie model.
   - Headers: `Referer: https://www.bilibili.com/`, `Origin: https://www.bilibili.com`, browser-class `User-Agent`.
   - Query/body: none; no WBI signature on this navigation request.
   - Relevant response paths: `$.code`, `$.message`, `$.data.isLogin`, `$.data.mid`, `$.data.vipStatus`, `$.data.vipType`, `$.data.wallet.coupon_balance`.
   - ToolPro accepts login only when `code == 0 && data.isLogin == true`.
   - **VIP type is valid only when `vipStatus == 1`.** Effective types: `0` none/expired, `1` monthly VIP, `2` annual VIP.
   - `VipPrivilege` and `Charge`: effective type must be `2`.
   - `VipBigPoint` and `MangaPrivilege`: effective type must be nonzero.
   - `Manga`: no VIP requirement.

   Sources: [`INavApi.cs`][nav-api], [`UserInfo.cs`][user-info], [`AccountDomainService.cs`][account-domain], [`BiliCookie.cs`][cookie].

2. Header/auth terminology used below:
   - **Cookie:** `Cookie: SESSDATA=...` with account cookies; mutation CSRF values come from cookie `bili_jct`.
   - **Task Referer:** `https://big.bilibili.com/mobile/bigPoint/task`.
   - **Index Referer:** `https://big.bilibili.com/mobile/index`.
   - **App request:** captured mainland Android client protocol with `access_key`, Android parameters, and BiliDroid-class UA.
   - **Captured headers are not proven mandatory.** Bruno captures include device/session/risk headers absent from the C# client. Their minimum required subset is `[UNCONFIRMED]`.
   - **No endpoint below is documented as requiring WBI signing.** ToolPro’s WBI handler activates only when a request contains `wts`; these task request models do not contain it.

3. **Important implementation discrepancy:** ToolPro’s current `IApiApi`, `IMangaApi`, and `IShowApi` registrations use the **browser UA**, not `Security.UserAgentApp`. `BaseAppRequest.access_key` and `.appkey` are commented out, and its nullable `sign` has no app-signing producer in the inspected request pipeline. Thus the current C# workflows attempt cookie requests even where Bruno documents an app protocol. This is source behavior, **not evidence that all cookie variants currently succeed**.

   Sources: [`ServiceCollectionExtension.cs`][agent-registration], [`BaseAppRequest.cs`][base-app], [`WridEncryptionDelegatingHandler.cs`][wbi-handler], [`bruno/environments/default.yml`][bruno-env].

4. Shared errors, when returned by an endpoint:
   
   | `code` | Meaning |
   |---|---|
   | `0` | API-level success; inspect endpoint-specific result/state too |
   | `-2` | Invalid access key |
   | `-3` | Invalid API signing key |
   | `-101` | Not logged in |
   | `-111` | CSRF validation failed |
   | `-352` | Risk validation failed; API-collect associates this with invalid UA/WBI parameters |
   | `-400` | Invalid request |
   | `-401` | Unauthorized/illegal request |
   | `-403` | Insufficient permission |
   | `-412` | Request intercepted; API-collect identifies client-IP risk control |
   | `-500` | Server error |
   
   These are common error meanings, **not a claim that each endpoint returns every value**. Endpoint-specific eligibility error numbers absent from the sources remain `[UNCONFIRMED]`.

   Source: API-collect [`docs/misc/errcode.md`][errors].

## 1. `VipPrivilege`

### Numbered flow

1. Check `VipPrivilegeConfig.IsEnable`; disabled log: `已配置为关闭，跳过`.
2. Login through navigation endpoint.
3. Require active **annual VIP**. Otherwise log: `普通会员和月度大会员每月不赠送B币券，不需要领取权益喽`.
4. Claim **type `1`**, then **type `2`**, regardless of the first result.
5. If either claim returns `code == 0`, refresh account information through navigation again.
6. Current ToolPro does **not** query privilege status before these claims. The status endpoint is used elsewhere for daily experience.

Sources: [`VipPrivilegeTaskAppService.cs`][priv-app], [`VipPrivilegeDomainService.cs`][priv-domain].

### Endpoints

| Endpoint | Auth / headers | Parameters and encoding | Relevant response / codes |
|---|---|---|---|
| **GET `https://api.bilibili.com/x/vip/privilege/my`** | Cookie `SESSDATA`; API-collect also documents `access_key` authentication. ToolPro sets Task Referer and browser UA. No explicit Origin. | Cookie mode: no params. App mode: `access_key` as authentication parameter. | `$.code`; `$.data.list[*].type`, `.state`, `.expire_time`, `.vip_type`, `.next_receive_days`, `.period_end_unix`; `$.data.is_vip`, `.vip_status`, `.vip_type`, `.vip_is_annual`, `.vip_is_month`, `.is_overdue_vip`. Codes: `0` success, `-101` not logged in, `-400` invalid request. |
| **POST `https://api.bilibili.com/x/vip/privilege/receive`** | Cookie `SESSDATA` + `csrf=bili_jct`. ToolPro declares no endpoint-specific Referer/Origin; browser UA inherited. | API-collect: **`application/x-www-form-urlencoded`** body `type` integer, `csrf` string. ToolPro instead puts both in the **URL query** and sends POST with no modeled body. | `$.code`, `$.message`; `0` success; `-101` login; `-111` CSRF; `-400` bad request; `69800` network busy; **`69801` already claimed this benefit**. ToolPro logs every nonzero response as failure, including already claimed. |

Sources: [`IApiApi.cs`][api], API-collect [`docs/vip/info.md`][collect-vip-info], [`docs/vip/action.md`][collect-vip-action].

### Full confirmed type table

**ToolPro’s entire `VipPrivilegeType` enum contains only `1` and `2`.** IDs `3` onward come from API-collect, not additional ToolPro enum members.

| `type` | Meaning | Claimed by ToolPro `VipPrivilege`? |
|---:|---|---|
| 1 | B-coin voucher / B币券 | Yes, first |
| 2 | Membership benefits; API-collect identifies 会员购优惠券 | Yes, second |
| 3 | Manga benefit coupon / 漫画福利券 | No |
| 4 | Membership-shopping free-shipping coupon / 会员购包邮券 | No |
| 5 | Manga-shop coupon / 漫画商城优惠券 | No |
| 6 | Dress trial card / 装扮体验卡 | No |
| 7 | Classroom coupon / 课堂优惠券 | No |
| 8 | Game gift-box status; documented daily, `state` fixed at `1` in that document | No; not a normal monthly claim through `/privilege/receive` |
| 9 | Daily +10 experience claim status | Separate `/x/vip/experience/add` flow |
| 14–20 | Present in the preserved status-response example; names and exact receive eligibility **`[UNCONFIRMED]`** | No |

Privilege `state`: **`0` unclaimed, `1` claimed, `2` prerequisite incomplete**. Do not assume array indices equal type IDs.

Sources: [`VipPrivilegeType.cs`][priv-types], API-collect [`docs/vip/info.md`][collect-vip-info].

### Date/configuration and logs

- Current section: `VipPrivilegeConfig.IsEnable=true`, `VipPrivilegeConfig.Cron="0 0 1 * * ?"` — daily 01:00 in shipped Console/Web configurations.
- Legacy `DailyTaskConfig.DayOfReceiveVipPrivilege`: configuration guide documents `-1` → first day, `0` disabled, `1`–`31` chosen day; documented legacy default `1`.
- The domain-service date check is **commented out**. That old check allowed the target day **or the last day of the month**. The current `DailyTaskOptions` class no longer defines this property.
- **Current behavior has no day-of-month gate.** Use `VipPrivilegeConfig.Cron`.
- QingLong shipped cron: `0 1 * * *`, daily 01:00.
- Success per claim: `【领取】...`, `【结果】成功`.
- Nonzero response: `【结果】失败`, `【原因】{message}`.
- Suggested frequency: use the shipped daily status/claim cadence; benefit issuance itself is monthly. A status-aware independent client can distinguish `69801` from a new successful claim.

Sources: [`docs/configuration.md` §§3.3.6, 3.9.4][config-doc], [`VipPrivilegeOptions.cs`][priv-options], [`DailyTaskOptions.cs`][daily-options], [Console settings][console-settings], [Web settings][web-settings], [`bili_task_vip_privilege.sh`][priv-cron].

## 2. `Charge`

### Numbered flow

1. Check `ChargeTaskConfig.IsEnable`; login.
2. Require active **annual VIP**; otherwise `不是年度大会员，跳过`.
3. Read voucher balance from navigation response **`$.data.wallet.coupon_balance`**, defaulting missing wallet/balance to `0`.
4. If balance `< 2`, log `余额小于2，无法充电` and stop.
5. Resolve recipient:
   - Explicit `ChargeTaskConfig.AutoChargeUpId` other than blank/`-1`: use it.
   - Blank/`-1`: use **fallback UID `220893216`**.
6. Submit **the complete voucher balance** as decimal `bp_num`; use space charging with `otype=up`, `oid=up_mid`.
7. Success requires **both `$.code == 0` and `$.data.status == 4`**.
8. On success, submit a charge comment using returned `order_no`.
9. There is **no current day-of-month check inside the charge service**, no balance cap/clamping, and no self-recipient equality check.

Sources: [`ChargeTaskAppService.cs`][charge-app], [`ChargeDomainService.cs`][charge-domain], [`Config/Constants.cs`][config-constants].

### Wallet endpoints

| Endpoint | Auth / headers | Parameters and encoding | Relevant response / codes |
|---|---|---|---|
| **GET `https://api.bilibili.com/x/web-interface/nav`** — **actual ToolPro source of balance** | Shared login headers/auth above | None | `$.data.wallet.coupon_balance`; `$.data.vipStatus`, `$.data.vipType` |
| **POST `https://pay.bilibili.com/paywallet/wallet/getUserWallet`** — dedicated wallet documented by API-collect; **not called by the inspected charge workflow** | Cookie `SESSDATA`; document notes URL-escaping `,` and `*` in cookie value. Required Referer/Origin/UA restrictions **`[UNCONFIRMED]`**. | **JSON**: `platformType` required, documented default/valid example `3`; optional `panelType=3`, `timestamp` current Unix milliseconds, `traceId` same value as timestamp, `version="1.0"` | `$.code`, `$.errno`, `$.success`, `$.msg`, `$.showMsg`; `$.data.couponBalance`, `.availableBp`, `.totalBp`, `.defaultBp`, `.iosBp`, `.unavailableBp`, `.unavailableReason`. `code=0` success; `8006000004` missing parameter. Other errors may be in `errno`: `800501008` invalid/non-JSON body; `800501007` not logged in. |

Source: API-collect [`docs/wallet/info.md`][collect-wallet]; ToolPro [`UserInfo.cs`][user-info].

### Charge and comment endpoints

| Endpoint | Auth / headers | Required form body | Relevant response / codes |
|---|---|---|---|
| **POST `https://api.bilibili.com/x/ugcpay/web/v2/trade/elec/pay/quick`** | Cookie `SESSDATA`, `csrf=bili_jct`; **`Content-Type: application/x-www-form-urlencoded`**; `Referer: https://www.bilibili.com/`; `Origin: https://www.bilibili.com`; browser UA | `bp_num`: B-coin/shell amount, documented **2–9999**; `is_bp_remains_prior=true`: B-coin-balance priority flag; `up_mid`: recipient UID; `otype=up` for space, `archive` for video; `oid`: recipient UID for `up`, video AV ID for `archive`; `csrf` | `$.code`, `$.message`; `$.data.status`, `.msg`, `.order_no`, `.bp_num`, `.exp`, `.mid`, `.up_mid`. `code=0` does **not** alone mean payment success. `data.status=4` paid; `-2` below minimum; `-4` insufficient B coins. Root codes documented: `-101`, `-111`, `-400`, `-500`. |
| **POST `https://api.bilibili.com/x/ugcpay/trade/elec/message`** | Same Cookie, content type, Referer, Origin and UA | `order_id`: previous `data.order_no`; `message`: comment text; `csrf` | `$.code`, `$.message`; `0` success, `-101`, `-111`, `-400`, **`88203` duplicate comment**. ToolPro awaits the request but **does not examine its response code** before logging `【留言】{comment}`. |

Sources: [`IApiApi.cs`][api], [`ChargeRequest.cs`][charge-request], [`ChargeResponse.cs`][charge-response], [`ChargeCommentRequest.cs`][charge-comment], API-collect [`docs/electric/Bcoin.md`][collect-charge], [`docs/electric/charge_msg.md`][collect-charge-comment].

**Units:** current API uses `bp_num` directly in B coins/shells. **Do not multiply by ten.** The old `/x/ugcpay/trade/elec/pay/quick` used `elec_num` batteries; API-collect marks it expired. API-collect describes experience as `floor(bp_num)` when fractional. ToolPro’s success log uses requested balance as the experience amount rather than inspecting `data.exp`.

### Self-charge, configuration, frequency, logs

| Setting | Default / meaning |
|---|---|
| `ChargeTaskConfig.IsEnable` | `true` |
| `ChargeTaskConfig.Cron` | **`0 0 12 28 * ?`**, noon on day 28, Console/Web |
| `ChargeTaskConfig.AutoChargeUpId` | Console `"-1"`; Web `""`; both resolve to fallback **`220893216`** |
| `ChargeTaskConfig.ChargeComment` | `""`; blank/whitespace resolves to a randomly selected built-in encouragement comment, not an omitted comment |
| Legacy `DailyTaskConfig.DayOfAutoCharge` | Still named in command-line mapping, but absent from current `DailyTaskOptions` and unused by current charge service; current default/semantics **`[UNCONFIRMED]`** |
| Legacy `DailyTaskConfig.AutoChargeUpId` | Still named by old CLI mapping and Console Production JSON, but current service reads **`ChargeTaskConfig.AutoChargeUpId`** |

- Current source and configuration guide say **Bilibili no longer permits charging yourself**. The old Console inline comment saying `-1` means self is stale; current behavior uses the fallback.
- Historical API-collect examples show self-charging succeeding. **This conflicts with current ToolPro documentation** and must be treated as historical.
- A manually configured self UID is not intercepted locally. Current server error/status for self-charge is **`[UNCONFIRMED]`**.
- Configuration guide says Production overrides the recipient, but inspected `appsettings.Production.json` puts `341688380` under **`DailyTaskConfig.AutoChargeUpId`**, not the current charge section.
- QingLong shipped cron is **`0 12 * * *`**, daily noon — **different from Console/Web’s monthly day-28 schedule**.
- Suggested frequency for a successor preserving voucher-expiry intent: monthly day 28, after privilege claiming. Repeated same-day calls are not prevented by the charge service; each run uses its newly read balance.
- Success logs: `【充电结果】成功`, `【充值个数】{balance}个B币`, `经验+{balance} √`.
- Root failure logs response message; `code=0` with non-`4` status logs full response as failure.

Sources: [`ChargeTaskOptions.cs`][charge-options], [`ChargeDomainService.cs`][charge-domain], [`docs/configuration.md` §3.7][config-doc], [Console settings][console-settings], [Web settings][web-settings], [Production overrides][production-settings], [`bili_task_charge.sh`][charge-cron].

## 3. `VipBigPoint` — 大会员大积分

### Current ToolPro execution order

1. Check `VipBigPointConfig.IsEnable`; login; require **any active VIP**, monthly or annual. Non-VIP log: `当前不是大会员，跳过任务`.
2. **Experience accelerator:** query `/x/vip/privilege/my`, inspect **type `9`**; perform watch prerequisite if needed; claim `/x/vip/experience/add`.
3. **Sign:** query `/x/vip/vip_center/sign_in/three_days_sign`; if unsigned, POST `/pgc/activity/score/task/sign2`; query sign status again.
4. **Fetch task combine/list.**
5. One-time **`bonus`**: receive if necessary, complete.
6. One-time **`privilege`**: receive if necessary, complete.
7. **Receive every daily task whose `state == 0`**, including purchase tasks.
8. Execute daily handlers in this exact order:
   1. `dress-view`
   2. `vipmallview`
   3. `vipmallbuy` → `需购买，跳过`
   4. `animatetab`
   5. `filmtab`
   6. `ogvwatchnew`
   7. `tvodbuy` → `需购买，跳过`
   8. `dressbuyamount` → `需购买，跳过`
9. Fetch combine again and log final status.

**Current OGV behavior is not the full native app watch protocol:** ToolPro invokes generic `/pgc/activity/score/task/complete/v2` with `taskCode=ogvwatchnew`. Its `StartOgvWatchAsync` and `CompleteOgvWatchAsync` interface methods have **no HTTP attributes** and are not called by this flow. Its private `WatchBangumi` helper is also not called.

Sources: [`VipBigPointAppService.cs`][point-app], [`VipBigPointDomainService.cs`][point-domain], [`IApiApi.cs`][api].

### Task state handling

1. Match module by literal Chinese `module_title`: `福利任务`, `体验任务`, or `日常任务`.
2. Match item by `task_code`.
3. Missing item → log `任务失效`, stop that subtask.
4. `state == 3` → log `已完成，跳过`.
5. `state == 0` → receive; ToolPro catches/logs receive failures and still proceeds to completion.
6. On completion API `code == 0`, query combine again and confirm **`state == 3 && complete_times >= 1`**.
7. Confirmation log is hardcoded `成功，经验 +10`, even for tasks whose advertised reward differs.
8. The initial combine object is reused after “receive all daily missions”; it is not updated in memory. Consequently a subtask originally at `state=0` may receive again before completing.

Relevant task paths: `$.data.task_info.modules[*].module_title`, `.common_task_item[*].task_code`, `.state`, `.complete_times`, `.max_times`, `.vip_limit`, `.explain`, `.link`; point paths: `$.data.point_info.point`, `.expire_point`, `.expire_time`, `.expire_days`; monthly counters: `$.data.task_info.score_month`, `.score_limit`.

ToolPro explicitly marks **`$.data.task_info.sing_task_item` sign status unreliable** and uses the separate three-day-sign endpoint.

Sources: [`VipBigPointDomainService.cs`][point-domain], [`TaskInfo.cs`][task-model], [`bruno/.../task/combine.yml`][combine-bruno].

### Experience accelerator endpoints

| Endpoint | Auth / headers | Parameters / encoding | Response and behavior |
|---|---|---|---|
| **GET `https://api.bilibili.com/x/vip/privilege/my`** | Cookie, Task Referer; browser UA in current C# | None | Find `$.data.list[?(@.type==9)].state`: `2` watch incomplete, `0` ready/unclaimed, `1` claimed |
| **POST `https://api.bilibili.com/x/vip/experience/add`** | Cookie + `csrf`, Task Referer in C#; no explicit Origin. API-collect confirms Cookie authentication. Bruno separately captures an **access-key app variant** with Index Referer plus query suffix `?exp_symbol=release_version&oflAb=1` | C#: form `csrf`. Bruno app form additionally: `access_key`, `buvid`, `disable_rcmd=0`, `mobi_app=android`, `platform=android`, `statistics`; `appkey`, `ts`, `sign` are **disabled** in example | `$.code`, `$.message`, `$.data.is_grant`, `.type`. `0` granted; **`69198` already claimed**; **`6034005` task incomplete**; **`6034007` too frequent**; `-101`, `-111` |

- If type-9 state is `2`, ToolPro queries daily-task status and delegates to `WatchAndShareVideo`, then **immediately attempts experience redemption**.
- Source comment describes watching **one minute for +10 account experience**.
- State `1`: `大会员经验已兑换`.
- State `0`: `大会员经验未兑换`; successful root code logs `领取成功，经验+10 √`.
- ToolPro does not inspect `data.is_grant` on success.
- `DailyTaskConfig.IsWatchVideo` and `.IsShareVideo` affect the delegated daily-video operation; the configuration guide warns disabling watch can prevent this extra claim.

Sources: [`VipBigPointDomainService.cs`][point-domain], [`VipExperienceRequest.cs`][experience-request], [`bruno/.../experience/add.yml`][experience-bruno], API-collect [`docs/vip/action.md`][collect-vip-action], [`VipBigPointApiTests.cs`][point-tests]. The tests define accepted codes; they were not executed.

### Sign / three-day-sign endpoints

| Endpoint | Auth / headers | Parameters / encoding | Relevant result / codes |
|---|---|---|---|
| **GET `https://api.bilibili.com/x/vip/vip_center/sign_in/three_days_sign`** | C#: Cookie + Task Referer. Bruno: Cookie and `access_key`, Index Referer, app UA; appkey/sign disabled | C# query: `csrf` plus base app fields below. Bruno also has `access_key`, `device=phone`, `web_location=666.146` | `$.data.three_day_sign.signed`, `.count`, `.duration`, `.day`, `.score`, `.vip_score`, `.vip_status`, `.exp_value`, `.has_coupon`, `.received_coupon`, `.countdown`; `$.data.big_point.point`; sample root `code=0` |
| **POST `https://api.bilibili.com/pgc/activity/score/task/sign2`** — current | Cookie; Index Referer; **JSON** `Content-Type: application/json; charset=utf-8`; no `access_key` or app sign in captured/current request | Query **`mobi_app=android&csrf={bili_jct}&platform=android`**. JSON **`{"device":"phone","t":<Unix milliseconds>,"ts":<Unix seconds>}`**, `ts=t/1000` | `$.code`, `$.message`, `$.data.count`, `.countdown`, `.duration`, `.score`, `.vipScore`, `.vipStatus`, `.hasCoupon`; sample `code=0`. Exact duplicate/not-VIP codes **`[UNCONFIRMED]`** |
| **POST `https://api.bilibili.com/pgc/activity/score/task/sign`** — legacy | API-collect: **Cookie OR access_key**. Cookie requires a `*.bilibili.com` Referer and URL-escaped SESSDATA. Current C# marks obsolete. Bruno app example uses app UA and signed form | API-collect form: `access_key` required only for app auth; `csrf` optional. C# old body: `csrf`, `statistics` version `6.85.0`. Bruno additionally sends `appkey`, `disable_rcmd`, `mobi_app`, `platform`, `statistics`, `ts`, `sign` | `0` success; `-101` login; `-401` illegal request; `-403` permission |

**Three-day behavior:** this is a status/check-and-sign flow, not a separate “claim three-day reward” POST. If `three_day_sign.signed == true`, log `已完成，跳过` and status. Otherwise sign2 success requires root `code=0` and non-null data; log `签到成功`, then re-query points. The response model describes accumulating **three sign-in days within `duration` days**; sample `duration=7`, `exp_value=3`. Exact reward policy should come from response fields rather than a fixed three-day calendar or assumed annual-VIP requirement.

**C# base app query/body fields** where a request inherits `BaseAppRequest`:
`build="8451100"`, `disable_rcmd=0`, `mobi_app="android"`, `platform="android"`, `statistics='{"appId":1,"platform":3,"version":"8.45.1","abtest":""}'`, `t` current milliseconds, `ts=t/1000`, nullable `sign`; **no active `access_key` or `appkey` property**.

Sources: [`IApiApi.cs`][api], [`Sign2RequestPath.cs`][sign-path], [`Sign2Request.cs`][sign-body], [`ThreeDaySignRequest.cs`][three-request], [`ThreeDaySignResponse.cs`][three-response], [`BaseAppRequest.cs`][base-app], [`bruno/.../sign2.yml`][sign2-bruno], [`bruno/.../three_days_sign.yml`][three-bruno], [`bruno/.../sign.yml`][sign-bruno], API-collect [`docs/vip/clockin.md`][collect-sign].

### Combine / receive / generic complete / browsing endpoints

All URLs below are full **`api.bilibili.com`** URLs; current C# uses Cookie and Task Referer, no explicit Origin, browser UA.

| METHOD + URL | Request | Relevant response / authentication distinction |
|---|---|---|
| **GET `https://api.bilibili.com/x/vip_point/task/combine`** | C# query: base app fields + `csrf`, `buvid` from cookie `buvid3`, `brand=Samsung`, `channel=bili`, `containerName=AbstractWebActivity`, `device=phone` | Root `code=0` and non-null data required. Bruno captures Cookie + `access_key`, explicitly **no signature verification**, with app UA/device metadata. Response varies with `build`. |
| **POST `https://api.bilibili.com/pgc/activity/score/task/receive/v2`** | C#: **form `taskCode=<code>`**, camel-case spelling significant; no modeled csrf/access_key. Bruno adds `csrf`, `access_key`, app/device fields | `code=0`, example `data={}`, message `success`. For `ogvwatchnew`, Bruno explicitly documents no app signature; appkey/ts/sign disabled. Purchase receipt examples include appkey/ts/sign. Cookie-only sufficiency for every task **`[UNCONFIRMED]`**. |
| **POST `https://api.bilibili.com/pgc/activity/score/task/complete`** | C#: **JSON `{"taskCode":"bonus"}`** or `"privilege"` | Root `code=0` means completion call accepted; verify combine state. |
| **POST `https://api.bilibili.com/pgc/activity/score/task/complete/v2`** | C#: **form `taskCode=dress-view`** or `ogvwatchnew` | Root `code=0`; verify combine state. Current OGV shortcut’s server acceptance **`[UNCONFIRMED]`**. |
| **POST `https://api.bilibili.com/pgc/activity/deliver/task/complete`** — browse variant | C#: **form** base app fields + `position=jp_channel` for anime / `tv_channel` for film; `c_locale=zh_CN`, `s_locale=zh_CN`, `channel=bili`. Explicit **10-second delay before request**. | Bruno captures **signed access-key app request**, additionally `win_id=bigscore-animatetab` / `bigscore-filmtab`; example `code=0`. Current C# model omits `win_id`, appkey and access_key. |
| **POST `https://show.bilibili.com/api/activity/fire/common/event/dispatch`** | Cookie; **JSON `{"csrf":"<bili_jct>","eventId":"hevent_oy4b7h3epeb"}`** | Current C# has no endpoint-specific Referer/Origin; browser UA. Root `code=0` accepted, otherwise throws. No explicit 10-second wait in its mall handler. Required additional app auth/headers and duplicate error codes **`[UNCONFIRMED]`**. |

**Captured app metadata:** combine/receive examples also carry `bili_local_id`, device fingerprint/name/platform variants, `local_id`, `networkState`/`networkstate`, `osVer`, `sessionID`, `statusBarHeight`, `isPad`; these are captured client fields, not a source-proven minimum required parameter set.

**Captured H5/app headers:** `native_api_from: h5`, `buvid`, `bili-http-engine`, `x-bili-mid`, `x-bili-aurora-eid`, `x-bili-trace-id`, `x-bili-ticket`; some captures add `x-bili-metadata-legal-region: CN`, `x-bili-net-bin`, empty aurora-zone/gaia-vtoken. Current C# does not supply these endpoint headers. Their necessity is `[UNCONFIRMED]`; captured fixed account/ticket values must not become implementation constants.

Sources: [`IApiApi.cs`][api], [`IShowApi.cs`][show-api], [`ReceiveOrCompleteTaskRequest.cs`][task-request], [`GetCombineRequest.cs`][combine-request], [`ViewRequest.cs`][view-request], [`ViewVipMallRequest.cs`][mall-request], [`bruno/.../combine.yml`][combine-bruno], [`bruno/.../receive/v2/ogvwatchnew.yml`][ogv-receive], [`bruno/.../deliver/task/complete.yml`][browse-complete].

### Subtask meaning, limits, and actual completion choice

Values below are from the saved combine response; treat server-returned explanations/limits as authoritative if they change.

| Task code | Eligibility / reward / limit in example | ToolPro completion behavior |
|---|---|---|
| `bonus` | Active monthly VIP +100 big points / active annual VIP +200; once per account. Upgrading after claiming does not grant the difference. | Receive v2 → JSON generic complete |
| `privilege` | Browse VIP-rights page; +50; once per account; sample `vip_limit=0`, though outer ToolPro task still requires VIP | Receive v2 → JSON generic complete |
| `dress-view` | Browse dress-store home; +10; once; `vip_limit=1` | Receive v2 → form generic complete/v2 |
| `vipmallview` | Continuously browse mall **10 seconds**; +10 daily; leaving interrupts countdown | Receive v2 → mall event dispatch; no explicit local countdown |
| `vipmallbuy` | First eligible purchase after receiving that day: +10 per paid RMB, daily cap 2000 points; exclusions/refund rules in `explain` | Receives if state 0; **does not buy**, logs skip |
| `animatetab` | Continuous anime-tab browse 10 seconds; +10 daily | Receive v2 → wait 10 s → deliver complete, `position=jp_channel` |
| `filmtab` | Continuous film-tab browse 10 seconds; +10 daily | Receive v2 → wait 10 s → deliver complete, `position=tv_channel` |
| `ogvwatchnew` | Receive first; continuously watch one eligible OGV video **10 minutes**; +40 daily; switching videos resets timer; **mainland mobile app only** per response explanation | Current C# uses generic complete/v2; full captured app protocol below |
| `tvodbuy` | Buy one eligible paid film after receipt; +70 daily | Receives if state 0; logs purchase skip |
| `dressbuyamount` | Eligible dress-store purchase after receipt; +10 per paid RMB; cap 1000 daily | Receives if state 0; logs purchase skip |

Source: [`bruno/api.bilibili.com/x/vip_point/task/combine.yml`][combine-bruno], [`VipBigPointAppService.cs`][point-app].

### Full captured native OGV watch protocol

This protocol is documented in Bruno but **not wired into ToolPro’s current running task**.

1. **Receive**
   - **POST `https://api.bilibili.com/pgc/activity/score/task/receive/v2`**
   - Form `taskCode=ogvwatchnew`, `access_key`, `csrf`, Android base/device fields.
   - Cookie + access key in captured request; app UA, Task Referer, H5/app headers above.
   - Appkey/ts/sign **disabled**; document says no signature verification.
   - Success root `code=0`.

2. **Start**
   - **POST `https://api.bilibili.com/pgc/activity/deliver/material/receive`**
   - **Form:** `access_key`, `activity_code=""`, `build`, `c_locale=zh_CN`, `channel=bili`, `disable_rcmd=0`, `ep_id`, `season_id`, `mobi_app=android`, `platform=android`, `s_locale=zh_CN`, `statistics`, `spmid=united.player-video-detail.0.0`, `from_spmid=activity.h5.0.0` in Bruno.
   - Saved example: `ep_id=328482`, `season_id=12548` (`让子弹飞`); these are **example content IDs**, not a public config default.
   - Cookie + access key; no signature verification per Bruno.
   - Native headers include **`app-key: android64`**, `env: prod`, `buvid`, `fp_local`, `fp_remote`, `session_id`, app UA, `x-bili-mid`, risk/ticket/trace headers, `bili-http-engine: cronet`.
   - No Referer/Origin in this captured native request.
   - Relevant response:
     - `$.code == 0`
     - **`$.data.watch_count_down_cfg.task_id`**
     - **`$.data.watch_count_down_cfg.token`**
     - **`$.data.watch_count_down_cfg.milliseconds`**, sample `600000`
     - `.login`, `.complete_status_desc`
   - **Response-model discrepancy:** ToolPro `StartOgvWatchResponse` incorrectly models `task_id` and `token` directly under `data`; the Bruno example places them in `watch_count_down_cfg`.

3. **Watch continuously for returned countdown**, sample ten minutes. The captured completion is emitted after ten minutes; no source proves an immediately submitted completion is acceptable.

4. **Complete**
   - **POST `https://api.bilibili.com/pgc/activity/deliver/task/complete`**
   - Same native header class.
   - **Form:** `access_key`, `appkey`, `ts`, `sign`, `task_id`, `token`, **`timestamp`** milliseconds, **`task_sign`**, `build`, locales, channel, Android/platform/statistics/disable_rcmd fields.
   - Bruno explicitly says **`task_sign` is mandatory**:
     1. Exclude `task_sign` and `sign`.
     2. Sign the remaining parameters using normal app signing; assign to `task_sign`.
     3. Sign again with `task_sign` included; assign result to `sign`.
   - Normal app signing: sort parameter names, URL-encode key/value pairs, join with `&`, append matching `appsec`, MD5.
   - Bruno’s configured logged-in pair: `appkey=1d8b6e7d45233436`, `appsec=560c52ccd288fed045859ed18bffd973`.
   - Document says completion succeeds **once**; second/subsequent calls return **`400`**. Whether that means HTTP 400 or root business `code=400` is **`[UNCONFIRMED]`** because the file provides no response example.
   - Query combine again to establish credited task state.

**Host inconsistency:** some Bruno prose lists `app.bilibili.com` for these three calls, but the actual requests inherit **`https://api.bilibili.com`**, live in the `api.bilibili.com` folder, and the receipt file’s flow uses that host. Both source claims are recorded; use of the `app.bilibili.com` alias is `[UNCONFIRMED]`.

Sources: [`bruno/.../receive/v2/ogvwatchnew.yml`][ogv-receive], [`bruno/.../deliver/material/receive.yml`][ogv-start], [`bruno/.../deliver/task/complete-ogv.yml`][ogv-complete], [`bruno/api.bilibili.com/folder.yml`][bruno-api-folder], [`bruno/opencollection.yml`][bruno-root], [`bruno/lib/sign.js`][app-sign], [`StartOgvWatchResponse.cs`][ogv-response].

### Configuration, frequency, and logs

| Setting | Default / effect |
|---|---|
| `VipBigPointConfig.IsEnable` | `true` |
| `VipBigPointConfig.Cron` | **`0 7 1 * * ?`**, daily 01:07, Console/Web |
| `VipBigPointConfig.ViewBangumis` | **`"33378"`**, Detective Conan season ID; comma-separated season IDs supported; blank/`-1` → empty list |
| `DailyTaskConfig.IsWatchVideo` | Shipped `true`; affects delegated daily watch for +10 account-experience prerequisite |
| `DailyTaskConfig.IsShareVideo` | Shipped `true`; affects same delegated operation |
| `Security.UserAgentApp` | Android webview/BiliApp UA exposed in configuration, but not selected by current `IApiApi` registration |

- `ViewBangumis` is currently consumed only by the **uncalled private `WatchBangumi` helper**. Changing it does not change the active generic OGV-completion handler.
- That unused helper selects one configured season/episode and reports a randomized `905`–`1799` seconds through video heartbeat; **this is not the wired native countdown flow**.
- Suggested frequency: once daily; shipped QingLong expression **`7 1 * * *`**.
- Logs: `领取任务成功`, `领取任务失败`, `已完成`, `浏览完成`, `浏览失败`; state confirmation as described above.
- Final combine log labels big-point balance `当前经验` and uses `√` only for `state==3`; this label does not mean big points and account experience are the same currency.

Sources: [`VipBigPointOptions.cs`][point-options], [`VipBigPointDomainService.cs`][point-domain], [`VipBigPointCombine.cs`][point-model], [`docs/configuration.md` §§3.3.1, 3.6][config-doc], [Console settings][console-settings], [Web settings][web-settings], [`bili_task_vipBigPoint.sh`][point-cron].

## 4. `Manga`

### Numbered flow

1. Check `MangaTaskConfig.IsEnable`; login through navigation.
2. POST manga clock-in.
3. POST configured manga reading-history entry, unless `CustomComicId <= 0`.
4. No VIP eligibility check; no manga-content fetching, chapter selection, reading-duration reporting, or separate task-reward claim is implemented by this task.

Sources: [`MangaTaskAppService.cs`][manga-app], [`MangaDomainService.cs`][manga-domain], [`IMangaApi.cs`][manga-api].

### Endpoints

| Endpoint | Auth / headers | Parameters / encoding | Relevant response / errors |
|---|---|---|---|
| **POST `https://manga.bilibili.com/twirp/activity.v1.Activity/ClockIn`** | Cookie `SESSDATA`; API-collect also documents APP auth. ToolPro headers: **`Origin: https://manga.bilibili.com`**, host `manga.bilibili.com`, browser UA; no explicit Referer or csrf | **ToolPro:** query `platform=DailyTaskConfig.DevicePlatform`, no modeled body. **API-collect:** form `platform=android` required, optional `device=h5`. JSON `type`/`date` is documented for retroactive sign-in, not used by ToolPro. | `$.code`, **`$.msg`**, `$.data`; `code=0` success. Duplicate: **string `code="invalid_argument"`**, `msg="clockin clockin is duplicate"`, `meta.argument="clockin"`. ToolPro comment says duplicate causes **HTTP 400**. |
| **POST `https://manga.bilibili.com/twirp/bookshelf.v1.Bookshelf/AddHistory`** | Cookie; same Manga Origin/host/browser UA. No explicit csrf/access key | **ToolPro query:** `platform`, `comic_id` configured manga ID, `ep_id` configured chapter/episode ID; no modeled body/content type | ToolPro uses root `code==0` as success. Extra response fields, endpoint-specific errors, and any required body alternative **`[UNCONFIRMED]`**. |

- ToolPro clock-in catches **all exceptions** and logs `【签到结果】失败` / `今日已签到过，无法重复签到`, even if the exception had another cause. An independent implementation should preserve the observed distinction between a known duplicate response and an unknown transport error.
- Clock-in success: `【签到结果】成功`; nonzero numeric business response: failure + response message.
- Reading success: `【漫画阅读】成功`; otherwise failure + message.
- `AddHistory` is the **only reading endpoint ToolPro calls**. Treat its result as successful history submission; the sources do not establish full reading or reward settlement.
- Manga responses use `msg`; duplicate `code` can be a string. A response decoder limited to an integer `code` will not faithfully represent the documented duplicate response.

Sources: [`IMangaApi.cs`][manga-api], [`MangaDomainService.cs`][manga-domain], API-collect [`docs/manga/ClockIn.md`][collect-manga-clock].

### Configuration and frequency

| Setting | Default | Meaning |
|---|---|---|
| `MangaTaskConfig.IsEnable` | `true` | Entire manga task |
| `MangaTaskConfig.Cron` | **`0 0 14 * * ?`** | Daily 14:00 |
| `MangaTaskConfig.CustomComicId` | **`27355`** | Manga ID; `<=0` silently disables reading-history request |
| `MangaTaskConfig.CustomEpId` | **`381662`** | Chapter/episode ID; no additional local validity check |
| `DailyTaskConfig.DevicePlatform` | **`"android"`** | Sent as platform; options document `android` or `ios` |

Suggested frequency: once daily, matching Console/Web and QingLong **`0 14 * * *`**. There are no independent `IsSign`/`IsRead` switches in this task’s options class.

Sources: [`MangaTaskOptions.cs`][manga-options], [`DailyTaskOptions.cs`][daily-options], [Console settings][console-settings], [Web settings][web-settings], [`bili_task_manga.sh`][manga-cron].

## 5. `MangaPrivilege`

### Numbered flow

1. Check `MangaPrivilegeTaskConfig.IsEnable`; login.
2. Require **any active VIP**, monthly or annual; otherwise `不是会员，跳过`.
3. Call manga VIP benefit endpoint with **hardcoded `reason_id=1`**.
4. Root `code==0` **and non-null data** means success; log returned coupon amount.
5. No current day-of-month filter, privilege-status query, or local already-claimed check.

Sources: [`MangaPrivilegeTaskAppService.cs`][manga-priv-app], [`MangaDomainService.cs`][manga-domain].

### Endpoint

**POST `https://manga.bilibili.com/twirp/user.v1.User/GetVipReward`**

| Field | Specification |
|---|---|
| Auth | Cookie `SESSDATA`; no access_key/appkey/sign or csrf in ToolPro request |
| Headers | `Origin: https://manga.bilibili.com`; host `manga.bilibili.com`; browser UA; no explicit Referer |
| Parameters | **Query `reason_id=1`**, no modeled body/content type |
| Meaning | Source comment identifies `1` as claiming manga reading coupons / 漫读券; other reason IDs are not attempted |
| Relevant response | `$.code`, response message, **`$.data.amount`** |
| Success | `code==0 && data!=null` |
| Known duplicate / not-eligible codes | **`[UNCONFIRMED]`** |
| Logs | `【领取结果】成功`, `【获取】{amount}张漫读劵`; otherwise `【领取结果】失败`, `【原因】{message}` |

**Do not equate `reason_id=1` with privilege `type=1`.** They are different endpoint namespaces. A comment in `MangaDomainService` says reason IDs can be obtained from `/x/vip/privilege/my`, but the inspected privilege response model/documentation does not establish a `reason_id` mapping. That derivation is **`[UNCONFIRMED]`**; the only executable behavior confirmed is hardcoded `1`.

### Configuration and frequency

- `MangaPrivilegeTaskConfig.IsEnable=true`.
- `MangaPrivilegeTaskConfig.Cron="0 0 15 * * ?"` — daily 15:00.
- No exposed `ReasonId`, manga-benefit-type, or day-of-month option.
- Task label describes a **monthly** VIP benefit, but shipped scheduler runs daily; QingLong **`0 15 * * *`**.
- Suggested frequency: retain daily claim/check cadence if matching ToolPro; exact benefit renewal eligibility comes from server, not a locally enforced monthly day.

Sources: [`IMangaApi.cs`][manga-api], [`MangaVipRewardResponse.cs`][manga-reward], [`MangaPrivilegeTaskOptions.cs`][manga-priv-options], [Console settings][console-settings], [Web settings][web-settings], [`bili_task_manga_privilege.sh`][manga-priv-cron].

## 6. Cross-task configuration and scheduling details

| Configuration | Source-confirmed defaults / behavior |
|---|---|
| Every task section `.IsEnable` | Base class default **`true`**, shipped sections also `true`; disabled log `已配置为关闭，跳过` |
| Every task section `.Cron` | Base class default `null`; concrete shipped values listed above |
| Environment spelling | `Section__Property`; Console commonly prefixes `Ray_`, e.g. `Ray_ChargeTaskConfig__AutoChargeUpId` |
| Web cron syntax | **Quartz with seconds**, e.g. `0 7 1 * * ?`; QingLong shell comments use five-field Unix cron |
| `Security.IntervalSecondsBetweenRequestApi` | **Docs say `20`; current Console JSON says `10`; options-class default is `3`; Web JSON does not override it.** |
| Actual interval algorithm | For normal matching requests, random integer seconds in **`floor(N/2)` through `N` inclusive**, delayed before each request. The configuration guide’s description of a full minimum spacing does not match this implementation. |
| `Security.IntervalMethodTypes` | Default `"GET,POST"`; empty disables method matching; interval `<=0` disables delay |
| Explicit subtask waits | Anime/film browse handlers wait **10 s**; current mall handler has no explicit 10 s wait; current OGV shortcut has no ten-minute wait |
| `Security.RandomSleepMaxMin` | Console shipped `0`; options-class default `10`; configuration guide describes Console start delay |
| `Security.UserAgent` | Shipped desktop Chrome/Edge-class UA; inherited by these current API clients |
| `Security.UserAgentApp` | Exposed Android BiliApp UA, but unused by these client registrations |
| `Security.WebProxy` | Empty default; proxy configuration shared across clients |

Cron wall-clock timezone is not established by the inspected task/configuration files: **`[UNCONFIRMED]`**. ToolPro’s date checks/logs use `DateTime.Today` where present, so host-local calendar assumptions should not silently be treated as a documented Bilibili renewal timezone.

Sources: [`BaseConfigOptions.cs`][base-options], [`SecurityOptions.cs`][security-options], [`IntervalDelegatingHandler.cs`][interval-handler], [`docs/configuration.md`][config-doc], [Console settings][console-settings], [Web settings][web-settings].

[nav-api]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Interfaces/INavApi.cs
[user-info]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/NavApi/UserInfo.cs
[account-domain]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/AccountDomainService.cs
[cookie]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliCookie.cs
[agent-registration]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/Extensions/ServiceCollectionExtension.cs
[base-app]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/BaseAppRequest.cs
[wbi-handler]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/WridEncryptionDelegatingHandler.cs
[bruno-env]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/bruno/environments/default.yml
[errors]: https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/misc/errcode.md
[api]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Interfaces/IApiApi.cs
[priv-app]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Application/VipPrivilegeTaskAppService.cs
[priv-domain]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/VipPrivilegeDomainService.cs
[priv-types]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/VipPrivilegeType.cs
[collect-vip-info]: https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/vip/info.md
[collect-vip-action]: https://goooler.github.io/bilibili-API-collect/docs/vip/action.html
[config-doc]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/docs/configuration.md
[priv-options]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Config/Options/VipPrivilegeOptions.cs
[daily-options]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Config/Options/DailyTaskOptions.cs
[console-settings]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Console/appsettings.json
[web-settings]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Web/appsettings.json
[production-settings]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Console/appsettings.Production.json
[priv-cron]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/platforms/qinglong/DefaultTasks/bili_task_vip_privilege.sh
[charge-app]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Application/ChargeTaskAppService.cs
[charge-domain]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/ChargeDomainService.cs
[config-constants]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Config/Constants.cs
[collect-wallet]: https://goooler.github.io/bilibili-API-collect/docs/wallet/info.html
[charge-request]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/Charge/ChargeRequest.cs
[charge-response]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/Charge/ChargeResponse.cs
[charge-comment]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/Charge/ChargeCommentRequest.cs
[collect-charge]: https://goooler.github.io/bilibili-API-collect/docs/electric/Bcoin.html
[collect-charge-comment]: https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/electric/charge_msg.md
[charge-options]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Config/Options/ChargeTaskOptions.cs
[charge-cron]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/platforms/qinglong/DefaultTasks/bili_task_charge.sh
[point-app]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Application/VipBigPointAppService.cs
[point-domain]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/VipBigPointDomainService.cs
[task-model]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/VipBigPoint/TaskInfo.cs
[combine-bruno]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/bruno/api.bilibili.com/x/vip_point/task/combine.yml
[experience-request]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/VipBigPoint/VipExperienceRequest.cs
[experience-bruno]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/bruno/api.bilibili.com/x/vip/experience/add.yml
[point-tests]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/test/Ray.BiliBiliTool.Agent.FunctionalTests/VipBigPointApiTests.cs
[sign-path]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/VipBigPoint/Sign2RequestPath.cs
[sign-body]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/VipBigPoint/Sign2Request.cs
[three-request]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/VipBigPoint/ThreeDaysSign/ThreeDaySignRequest.cs
[three-response]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/VipBigPoint/ThreeDaysSign/ThreeDaySignResponse.cs
[sign2-bruno]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/bruno/api.bilibili.com/pgc/activity/score/task/sign2.yml
[three-bruno]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/bruno/api.bilibili.com/x/vip/vip_center/sign_in/three_days_sign.yml
[sign-bruno]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/bruno/api.bilibili.com/pgc/activity/score/task/sign.yml
[collect-sign]: https://goooler.github.io/bilibili-API-collect/docs/vip/clockin.html
[show-api]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Interfaces/IShowApi.cs
[task-request]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/VipBigPoint/ReceiveOrCompleteTaskRequest.cs
[combine-request]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/VipBigPoint/GetCombineRequest.cs
[view-request]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/VipBigPoint/ViewRequest.cs
[mall-request]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ShowApi/ViewVipMallRequest.cs
[ogv-receive]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/bruno/api.bilibili.com/pgc/activity/score/task/receive/v2/ogvwatchnew.yml
[browse-complete]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/bruno/api.bilibili.com/pgc/activity/deliver/task/complete.yml
[ogv-start]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/bruno/api.bilibili.com/pgc/activity/deliver/material/receive.yml
[ogv-complete]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/bruno/api.bilibili.com/pgc/activity/deliver/task/complete-ogv.yml
[bruno-api-folder]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/bruno/api.bilibili.com/folder.yml
[bruno-root]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/bruno/opencollection.yml
[app-sign]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/bruno/lib/sign.js
[ogv-response]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/VipBigPoint/StartOgvWatchResponse.cs
[point-options]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Config/Options/VipBigPointOptions.cs
[point-model]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/VipBigPoint/VipBigPointCombine.cs
[point-cron]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/platforms/qinglong/DefaultTasks/bili_task_vipBigPoint.sh
[manga-app]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Application/MangaTaskAppService.cs
[manga-domain]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/MangaDomainService.cs
[manga-api]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Interfaces/IMangaApi.cs
[collect-manga-clock]: https://goooler.github.io/bilibili-API-collect/docs/manga/ClockIn.html
[manga-options]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Config/Options/MangaTaskOptions.cs
[manga-cron]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/platforms/qinglong/DefaultTasks/bili_task_manga.sh
[manga-priv-app]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Application/MangaPrivilegeTaskAppService.cs
[manga-reward]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/MangaApi/MangaVipRewardResponse.cs
[manga-priv-options]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Config/Options/MangaPrivilegeTaskOptions.cs
[manga-priv-cron]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/platforms/qinglong/DefaultTasks/bili_task_manga_privilege.sh
[base-options]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Config/Options/BaseConfigOptions.cs
[security-options]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Config/Options/SecurityOptions.cs
[interval-handler]: https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/HttpClientDelegatingHandlers/IntervalDelegatingHandler.cs