package bili

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"net/url"
	"strconv"
	"time"
)

const liveAPI = "https://api.live.bilibili.com"

func liveHeader() map[string]string {
	return map[string]string{"Referer": "https://live.bilibili.com/", "Origin": "https://live.bilibili.com"}
}

// LiveBootstrap 取得直播设备 Cookie，由调用方合并到账号。
func (c *Client) LiveBootstrap(ctx context.Context) (map[string]string, error) {
	resp, body, err := c.send(ctx, Request{URL: liveAPI + "/news/v1/notice/recom", Query: url.Values{"product": {"live"}}})
	if err != nil {
		return nil, err
	}
	if err := decodeEnvelope(body, "api.live.bilibili.com/news/v1/notice/recom", nil); err != nil {
		return nil, err
	}
	cookies := map[string]string{}
	for _, ck := range resp.Cookies() {
		cookies[ck.Name] = ck.Value
	}
	return cookies, nil
}

type LiveMedal struct {
	Medal struct {
		TargetID int64  `json:"target_id"`
		ID       int64  `json:"medal_id"`
		Level    int    `json:"level"`
		Name     string `json:"medal_name"`
	} `json:"medal"`
	Room struct {
		ID           int64 `json:"room_id"`
		LivingStatus int   `json:"living_status"`
	} `json:"room_info"`
	Anchor struct {
		Name string `json:"nick_name"`
	} `json:"anchor_info"`
}

type LiveMedalPanel struct {
	List    []LiveMedal `json:"list"`
	Special []LiveMedal `json:"special_list"`
	Page    struct {
		Total int  `json:"total_page"`
		More  bool `json:"has_more"`
	} `json:"page_info"`
}

func (c *Client) LiveMedalPanel(ctx context.Context, page int) (*LiveMedalPanel, error) {
	var d *LiveMedalPanel
	err := c.Do(ctx, Request{URL: liveAPI + "/xlive/app-ucenter/v1/fansMedal/panel", Query: url.Values{"page": {strconv.Itoa(page)}, "page_size": {"10"}}, Header: liveHeader()}, &d)
	if err == nil && d == nil {
		err = liveMissing("粉丝牌列表")
	}
	return d, err
}

type LiveMedalTask struct {
	Title    string `json:"title"`
	Subtitle string `json:"sub_title"`
	Type     string `json:"jump_type"`
	Done     bool   `json:"is_done"`
}

type LiveMedalProgress struct {
	Lighted bool            `json:"is_lighted"`
	Full    bool            `json:"reach_free_intimacy_limit"`
	Tasks   []LiveMedalTask `json:"task_info"`
}

func (c *Client) LiveMedalProgress(ctx context.Context, target int64) (*LiveMedalProgress, error) {
	var d *LiveMedalProgress
	err := c.Do(ctx, Request{URL: liveAPI + "/xlive/app-ucenter/v1/fansMedal/GetActivatedMedalInfo", Query: url.Values{"target_id": {strconv.FormatInt(target, 10)}, "csrf": {c.CSRF()}, "web_location": {"444.260"}}, Header: liveHeader()}, &d)
	if err == nil && d == nil {
		err = liveMissing("粉丝牌进度")
	}
	return d, err
}

type LiveRoomInfo struct {
	ID       int64 `json:"room_id"`
	UID      int64 `json:"uid"`
	AreaID   int64 `json:"area_id"`
	ParentID int64 `json:"parent_area_id"`
	Status   int   `json:"live_status"`
}

func (c *Client) LiveRoomInfo(ctx context.Context, room int64) (*LiveRoomInfo, error) {
	var d *LiveRoomInfo
	err := c.Get(ctx, liveAPI+"/room/v1/Room/get_info", url.Values{"room_id": {strconv.FormatInt(room, 10)}, "from": {"room"}}, &d)
	if err == nil && d == nil {
		err = liveMissing("直播间状态")
	}
	return d, err
}

func (c *Client) LiveLike(ctx context.Context, room, anchor int64, clicks int) error {
	return c.Do(ctx, Request{URL: liveAPI + "/xlive/app-ucenter/v1/like_info_v3/like/likeReportV3", Form: url.Values{"click_time": {strconv.Itoa(clicks)}, "room_id": {strconv.FormatInt(room, 10)}, "uid": {c.Mid()}, "anchor_id": {strconv.FormatInt(anchor, 10)}, "csrf_token": {c.CSRF()}}, Header: liveHeader()}, nil)
}

func (c *Client) LiveDanmaku(ctx context.Context, room int64, text string, now time.Time) error {
	return c.PostForm(ctx, liveAPI+"/msg/send", url.Values{"bubble": {"0"}, "msg": {text}, "color": {"16777215"}, "mode": {"1"}, "fontsize": {"25"}, "rnd": {strconv.FormatInt(now.Unix(), 10)}, "roomid": {strconv.FormatInt(room, 10)}, "csrf_token": {c.CSRF()}}, nil)
}

// LiveHeartbeatInput 的字段顺序就是心跳签名的 JSON 顺序。
type LiveHeartbeatInput struct {
	ParentID int64  `json:"parent_id"`
	AreaID   int64  `json:"area_id"`
	Sequence int    `json:"seq_id"`
	RoomID   int64  `json:"room_id"`
	Buvid    string `json:"buvid"`
	UUID     string `json:"uuid"`
	ETS      int64  `json:"ets"`
	Interval int    `json:"time"`
	TS       int64  `json:"ts"`
}

// LiveHeartbeatSign 按服务端给出的顺序串联 HMAC；下一轮的输入是小写十六进制文本。
func LiveHeartbeatSign(in LiveHeartbeatInput, key string, rules []int) (string, error) {
	body, err := json.Marshal(struct {
		Platform string `json:"platform"`
		LiveHeartbeatInput
	}{"web", in})
	if err != nil {
		return "", err
	}
	result := string(body)
	for _, rule := range rules {
		var algorithm func() hash.Hash
		switch rule {
		case 0:
			algorithm = md5.New
		case 1:
			algorithm = sha1.New
		case 2:
			algorithm = sha256.New
		case 3:
			algorithm = sha256.New224
		case 4:
			algorithm = sha512.New
		case 5:
			algorithm = sha512.New384
		default:
			return "", liveMissing("心跳签名规则不受支持")
		}
		mac := hmac.New(algorithm, []byte(key))
		_, _ = mac.Write([]byte(result))
		result = hex.EncodeToString(mac.Sum(nil))
	}
	return result, nil
}

// LiveWatchUUID 每次观看会话使用一个设备 UUID。
func LiveWatchUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

type LiveHeartbeatState struct {
	Interval  int    `json:"heartbeat_interval"`
	Key       string `json:"secret_key"`
	Rules     []int  `json:"secret_rule"`
	Timestamp int64  `json:"timestamp"`
}

func (c *Client) liveHeartbeat(ctx context.Context, endpoint string, anchor int64, in LiveHeartbeatInput, state *LiveHeartbeatState) (*LiveHeartbeatState, error) {
	id, _ := json.Marshal([4]int64{in.ParentID, in.AreaID, int64(in.Sequence), in.RoomID})
	device, _ := json.Marshal([2]string{in.Buvid, in.UUID})
	form := url.Values{"id": {string(id)}, "ruid": {strconv.FormatInt(anchor, 10)}, "ts": {strconv.FormatInt(in.TS, 10)}, "ua": {WebUA}, "csrf": {c.CSRF()}, "csrf_token": {c.CSRF()}, "visit_id": {""}, "device": {string(device)}, "web_location": {"444.8"}}
	if state == nil {
		form.Set("is_patch", "0")
		form.Set("heart_beat", "[]")
	} else {
		s, err := LiveHeartbeatSign(in, state.Key, state.Rules)
		if err != nil {
			return nil, err
		}
		form.Set("s", s)
		form.Set("ets", strconv.FormatInt(in.ETS, 10))
		form.Set("benchmark", state.Key)
		form.Set("time", strconv.Itoa(in.Interval))
		form.Set("trackid", "-99998")
	}
	form, err := c.SignWBI(ctx, form)
	if err != nil {
		return nil, err
	}
	var d *LiveHeartbeatState
	err = c.PostForm(ctx, "https://live-trace.bilibili.com/xlive/data-interface/v1/x25Kn/"+endpoint, form, &d)
	if err == nil && (d == nil || d.Interval < 1 || d.Interval > 300 || d.Key == "") {
		err = liveMissing("心跳间隔或密钥不对")
	}
	return d, err
}

func (c *Client) LiveEnter(ctx context.Context, anchor int64, in LiveHeartbeatInput) (*LiveHeartbeatState, error) {
	return c.liveHeartbeat(ctx, "E", anchor, in, nil)
}

func (c *Client) LiveHeartbeat(ctx context.Context, anchor int64, in LiveHeartbeatInput, state LiveHeartbeatState) (*LiveHeartbeatState, error) {
	return c.liveHeartbeat(ctx, "X", anchor, in, &state)
}

func liveMissing(message string) error {
	return &Error{Code: CodeBadResponse, Message: "直播接口返回的数据不完整：" + message}
}

type LiveArea struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (c *Client) LiveAreas(ctx context.Context) ([]LiveArea, error) {
	var d *struct {
		Data []LiveArea `json:"data"`
	}
	err := c.Get(ctx, liveAPI+"/xlive/web-interface/v1/index/getWebAreaList", url.Values{"source_id": {"2"}}, &d)
	if err != nil {
		return nil, err
	}
	if d == nil || d.Data == nil {
		return nil, liveMissing("直播分区")
	}
	return d.Data, nil
}

type LiveLotteryRoom struct {
	ID      int64           `json:"roomid"`
	UID     int64           `json:"uid"`
	Name    string          `json:"uname"`
	Pendant json.RawMessage `json:"pendant_info"`
}

// HasLotteryPendant 容忍空值、数组和标量，只接受文档列出的天选挂件。
func (r LiveLotteryRoom) HasLotteryPendant() bool {
	var entries map[string]json.RawMessage
	if json.Unmarshal(r.Pendant, &entries) != nil {
		return false
	}
	var pendant struct {
		ID int `json:"pendent_id"`
	}
	if json.Unmarshal(entries["2"], &pendant) != nil {
		return false
	}
	return pendant.ID == 504 || pendant.ID == 1432
}

func (c *Client) LiveLotteryRooms(ctx context.Context, area int64, page int) ([]LiveLotteryRoom, error) {
	var d *struct {
		List []LiveLotteryRoom `json:"list"`
	}
	err := c.Do(ctx, Request{URL: liveAPI + "/room/v3/area/getRoomList", Query: url.Values{"platform": {"web"}, "parent_area_id": {strconv.FormatInt(area, 10)}, "area_id": {"0"}, "sort_type": {"online"}, "page": {strconv.Itoa(page)}, "page_size": {"99"}}, Header: liveHeader()}, &d)
	if err != nil {
		return nil, err
	}
	if d == nil || d.List == nil {
		return nil, liveMissing("直播间列表")
	}
	return d.List, nil
}

type LiveLottery struct {
	ID          int64  `json:"id"`
	RoomID      int64  `json:"room_id"`
	Status      int    `json:"status"`
	Award       string `json:"award_name"`
	RequireType *int   `json:"require_type"`
	GiftID      int64  `json:"gift_id"`
	GiftNum     int    `json:"gift_num"`
	GiftPrice   int64  `json:"gift_price"`
}

func (c *Client) LiveLotteryCheck(ctx context.Context, room int64) (*LiveLottery, error) {
	var d *LiveLottery
	err := c.Do(ctx, Request{URL: liveAPI + "/xlive/lottery-interface/v1/Anchor/Check", Query: url.Values{"roomid": {strconv.FormatInt(room, 10)}}, Header: liveHeader()}, &d)
	return d, err
}

var liveLotteryVisit = func() string {
	// 首位为 1–9，中间十个小写字符，末位为 0；进程内复用。
	var b [11]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	for i := 1; i < len(b); i++ {
		b[i] = 'a' + b[i]%26
	}
	return string('1'+b[0]%9) + string(b[1:]) + "0"
}()

func (c *Client) LiveLotteryJoin(ctx context.Context, lottery LiveLottery) error {
	return c.PostForm(ctx, liveAPI+"/xlive/lottery-interface/v1/Anchor/Join", url.Values{"id": {strconv.FormatInt(lottery.ID, 10)}, "gift_id": {strconv.FormatInt(lottery.GiftID, 10)}, "gift_num": {strconv.Itoa(lottery.GiftNum)}, "platform": {"pc"}, "csrf_token": {c.CSRF()}, "visit_id": {liveLotteryVisit}}, nil)
}
