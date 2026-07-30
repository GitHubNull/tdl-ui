package services

import "testing"

func TestParseRangeHeader(t *testing.T) {
	const size = 10 << 20 // 10 MiB

	cases := []struct {
		name       string
		header     string
		size       int64
		wantStart  int64
		wantEnd    int64
		wantStatus rangeStatus
	}{
		{"无 Range 头", "", size, 0, 0, rangeNone},
		{"开放式起始", "bytes=0-", size, 0, size - 1, rangePartial},
		{"闭区间", "bytes=100-199", size, 100, 199, rangePartial},
		{"末尾偏移到文件尾", "bytes=1048576-", size, 1 << 20, size - 1, rangePartial},
		{"末尾 n 字节", "bytes=-1024", size, size - 1024, size - 1, rangePartial},
		{"末尾 n 超过文件长度取全量", "bytes=-99999999", size, 0, size - 1, rangePartial},
		{"结束越界收敛到末字节", "bytes=100-99999999", size, 100, size - 1, rangePartial},
		{"起始越界不可满足", "bytes=10485760-", size, 0, 0, rangeUnsatisfiable},
		{"大小写不敏感", "BYTES=0-9", size, 0, 9, rangePartial},
		{"多段范围按无 Range 处理", "bytes=0-9,20-29", size, 0, 0, rangeNone},
		{"非 bytes 单位", "items=0-9", size, 0, 0, rangeNone},
		{"缺少连字符", "bytes=100", size, 0, 0, rangeNone},
		{"结束小于起始", "bytes=200-100", size, 0, 0, rangeNone},
		{"非数字", "bytes=abc-def", size, 0, 0, rangeNone},
		{"负起始", "bytes=-0", size, 0, 0, rangeNone},
		{"文件长度为零", "bytes=0-", 0, 0, 0, rangeNone},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end, status := parseRangeHeader(c.header, c.size)
			if status != c.wantStatus {
				t.Fatalf("状态预期 %d，实际 %d", c.wantStatus, status)
			}
			if status != rangePartial {
				return
			}
			if start != c.wantStart || end != c.wantEnd {
				t.Fatalf("范围预期 %d-%d，实际 %d-%d", c.wantStart, c.wantEnd, start, end)
			}
		})
	}
}

func TestClampRange(t *testing.T) {
	const size = 10 << 20

	cases := []struct {
		name                     string
		start, end, size, maxLen int64
		wantStart, wantEnd       int64
	}{
		{"上限截断", 0, size - 1, size, videoRangeMax, 0, videoRangeMax - 1},
		{"短于上限保持原样", 100, 199, size, videoRangeMax, 100, 199},
		{"结束越界收敛", 0, size + 1000, size, videoRangeMax, 0, videoRangeMax - 1},
		{"负起始归零", -50, 99, size, videoRangeMax, 0, 99},
		{"maxLen 为零不限制", 0, size - 1, size, 0, 0, size - 1},
		{"中段截断从起始算起", 1 << 20, size - 1, size, videoRangeMax, 1 << 20, (1 << 20) + videoRangeMax - 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end := clampRange(c.start, c.end, c.size, c.maxLen)
			if start != c.wantStart || end != c.wantEnd {
				t.Fatalf("预期 %d-%d，实际 %d-%d", c.wantStart, c.wantEnd, start, end)
			}
			if got := end - start + 1; c.maxLen > 0 && got > c.maxLen {
				t.Fatalf("响应长度 %d 超过上限 %d", got, c.maxLen)
			}
		})
	}
}

func TestContentTypeFor(t *testing.T) {
	cases := []struct {
		name string
		mime string
		file string
		want string
	}{
		{"文档 MIME 优先", "video/mp4", "clip.webm", "video/mp4"},
		{"空 MIME 回退扩展名", "", "clip.mp4", "video/mp4"},
		{"空白 MIME 回退扩展名", "   ", "clip.webm", "video/webm"},
		{"大写扩展名", "", "CLIP.MOV", "video/quicktime"},
		{"完整路径", "", `C:\downloads\a b\movie.m4v`, "video/mp4"},
		{"matroska", "", "movie.mkv", "video/x-matroska"},
		{"未知扩展名兜底", "", "movie.unknown-ext", "application/octet-stream"},
		{"无扩展名兜底", "", "movie", "application/octet-stream"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := contentTypeFor(c.mime, c.file); got != c.want {
				t.Fatalf("预期 %s，实际 %s", c.want, got)
			}
		})
	}
}

func TestVideoCachePartLRU(t *testing.T) {
	c := newVideoCache()

	// 写满上限后再写一段：最久未使用的第 0 段应被淘汰
	for i := 0; i <= videoPartCacheMax; i++ {
		c.putPart(videoPartKey(1, 2, int64(i)), []byte{byte(i)})
	}
	if _, ok := c.getPart(videoPartKey(1, 2, 0)); ok {
		t.Fatal("超出上限后最久未使用分段应被淘汰")
	}
	if _, ok := c.getPart(videoPartKey(1, 2, videoPartCacheMax)); !ok {
		t.Fatal("最新写入分段应命中")
	}

	// 访问会刷新热度：被访问过的分段在后续淘汰中留存
	hot := videoPartKey(1, 2, 1)
	if _, ok := c.getPart(hot); !ok {
		t.Fatal("第 1 段应仍在缓存")
	}
	c.putPart(videoPartKey(1, 2, 999), []byte{9})
	if _, ok := c.getPart(hot); !ok {
		t.Fatal("刚访问过的分段不应被淘汰")
	}

	// 空分段不入缓存，避免把服务端空响应固化
	c.putPart(videoPartKey(3, 4, 0), nil)
	if _, ok := c.getPart(videoPartKey(3, 4, 0)); ok {
		t.Fatal("空分段不应写入缓存")
	}

	c.Clear()
	if _, ok := c.getPart(videoPartKey(1, 2, 1)); ok {
		t.Fatal("Clear 后分段应全部清空")
	}
}

func TestVideoCacheMeta(t *testing.T) {
	c := newVideoCache()
	key := videoKey(-100123, 88)

	if _, ok := c.getMeta(key); ok {
		t.Fatal("未写入时不应命中")
	}

	c.putMeta(key, videoMeta{Size: 123, MIME: "video/mp4", Name: "a.mp4"})
	m, ok := c.getMeta(key)
	if !ok || m.Size != 123 || m.MIME != "video/mp4" {
		t.Fatalf("元信息命中异常: ok=%v meta=%+v", ok, m)
	}

	c.dropMeta(key)
	if _, ok := c.getMeta(key); ok {
		t.Fatal("dropMeta 后不应命中（file_reference 失效需重新解析）")
	}

	c.putMeta(key, videoMeta{Size: 1})
	c.Clear()
	if _, ok := c.getMeta(key); ok {
		t.Fatal("Clear 后元信息应清空")
	}
}

func TestIsFileRefExpired(t *testing.T) {
	if isFileRefExpired(nil) {
		t.Fatal("nil 错误不应判定为 file_reference 失效")
	}
	if !isFileRefExpired(errFileRefStub{}) {
		t.Fatal("FILE_REFERENCE_EXPIRED 应判定为失效")
	}
	if isFileRefExpired(errOtherStub{}) {
		t.Fatal("无关错误不应判定为失效")
	}
}

type errFileRefStub struct{}

func (errFileRefStub) Error() string {
	return "rpc error code 400: FILE_REFERENCE_EXPIRED"
}

type errOtherStub struct{}

func (errOtherStub) Error() string { return "rpc error code 420: FLOOD_WAIT (5)" }
