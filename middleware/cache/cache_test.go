// Special thanks to @codemicro for moving this to fiber core
// Original middleware: github.com/codemicro/fiber-cache
package cache

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/internal/storage/memory"
	"github.com/gofiber/fiber/v2/utils"
	"github.com/valyala/fasthttp"
)

func Test_Cache_CacheControl(t *testing.T) {
	app := fiber.New()

	app.Use(New(Config{
		CacheControl: true,
		Expiration:   10 * time.Second,
	}))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Hello, World!")
	})

	_, err := app.Test(httptest.NewRequest("GET", "/", nil))
	utils.AssertEqual(t, nil, err)

	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	utils.AssertEqual(t, nil, err)
	utils.AssertEqual(t, "public, max-age=10", resp.Header.Get(fiber.HeaderCacheControl))
}

func Test_Cache_Expired(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Use(New(Config{Expiration: 2 * time.Second}))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString(fmt.Sprintf("%d", time.Now().UnixNano()))
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	utils.AssertEqual(t, nil, err)
	body, err := ioutil.ReadAll(resp.Body)
	utils.AssertEqual(t, nil, err)

	// Sleep until the cache is expired
	time.Sleep(3 * time.Second)

	respCached, err := app.Test(httptest.NewRequest("GET", "/", nil))
	utils.AssertEqual(t, nil, err)
	bodyCached, err := ioutil.ReadAll(respCached.Body)
	utils.AssertEqual(t, nil, err)

	if bytes.Equal(body, bodyCached) {
		t.Errorf("Cache should have expired: %s, %s", body, bodyCached)
	}

	// Next response should be also cached
	respCachedNextRound, err := app.Test(httptest.NewRequest("GET", "/", nil))
	utils.AssertEqual(t, nil, err)
	bodyCachedNextRound, err := ioutil.ReadAll(respCachedNextRound.Body)
	utils.AssertEqual(t, nil, err)

	if !bytes.Equal(bodyCachedNextRound, bodyCached) {
		t.Errorf("Cache should not have expired: %s, %s", bodyCached, bodyCachedNextRound)
	}
}

func Test_Cache(t *testing.T) {
	app := fiber.New()
	app.Use(New())

	app.Get("/", func(c *fiber.Ctx) error {
		now := fmt.Sprintf("%d", time.Now().UnixNano())
		return c.SendString(now)
	})

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	utils.AssertEqual(t, nil, err)

	cachedReq := httptest.NewRequest("GET", "/", nil)
	cachedResp, err := app.Test(cachedReq)
	utils.AssertEqual(t, nil, err)

	body, err := ioutil.ReadAll(resp.Body)
	utils.AssertEqual(t, nil, err)
	cachedBody, err := ioutil.ReadAll(cachedResp.Body)
	utils.AssertEqual(t, nil, err)

	utils.AssertEqual(t, cachedBody, body)
}

func Test_Cache_WithSeveralRequests(t *testing.T) {
	app := fiber.New()

	app.Use(New(Config{
		CacheControl: true,
		Expiration:   10 * time.Second,
	}))

	app.Get("/:id", func(c *fiber.Ctx) error {
		return c.SendString(c.Params("id"))
	})

	for runs := 0; runs < 10; runs++ {
		for i := 0; i < 10; i++ {
			func(id int) {
				rsp, err := app.Test(httptest.NewRequest(http.MethodGet, fmt.Sprintf("/%d", id), nil))
				utils.AssertEqual(t, nil, err)

				defer rsp.Body.Close()

				idFromServ, err := ioutil.ReadAll(rsp.Body)
				utils.AssertEqual(t, nil, err)

				a, err := strconv.Atoi(string(idFromServ))
				utils.AssertEqual(t, nil, err)

				// SomeTimes,The id is not equal with a
				utils.AssertEqual(t, id, a)
			}(i)
		}
	}
}

func Test_Cache_Invalid_Expiration(t *testing.T) {
	app := fiber.New()
	cache := New(Config{Expiration: 0 * time.Second})
	app.Use(cache)

	app.Get("/", func(c *fiber.Ctx) error {
		now := fmt.Sprintf("%d", time.Now().UnixNano())
		return c.SendString(now)
	})

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	utils.AssertEqual(t, nil, err)

	cachedReq := httptest.NewRequest("GET", "/", nil)
	cachedResp, err := app.Test(cachedReq)
	utils.AssertEqual(t, nil, err)

	body, err := ioutil.ReadAll(resp.Body)
	utils.AssertEqual(t, nil, err)
	cachedBody, err := ioutil.ReadAll(cachedResp.Body)
	utils.AssertEqual(t, nil, err)

	utils.AssertEqual(t, cachedBody, body)
}

func Test_Cache_Invalid_Method(t *testing.T) {
	app := fiber.New()

	app.Use(New())

	app.Post("/", func(c *fiber.Ctx) error {
		return c.SendString(c.Query("cache"))
	})

	app.Get("/get", func(c *fiber.Ctx) error {
		return c.SendString(c.Query("cache"))
	})

	resp, err := app.Test(httptest.NewRequest("POST", "/?cache=123", nil))
	utils.AssertEqual(t, nil, err)
	body, err := ioutil.ReadAll(resp.Body)
	utils.AssertEqual(t, nil, err)
	utils.AssertEqual(t, "123", string(body))

	resp, err = app.Test(httptest.NewRequest("POST", "/?cache=12345", nil))
	utils.AssertEqual(t, nil, err)
	body, err = ioutil.ReadAll(resp.Body)
	utils.AssertEqual(t, nil, err)
	utils.AssertEqual(t, "12345", string(body))

	resp, err = app.Test(httptest.NewRequest("GET", "/get?cache=123", nil))
	utils.AssertEqual(t, nil, err)
	body, err = ioutil.ReadAll(resp.Body)
	utils.AssertEqual(t, nil, err)
	utils.AssertEqual(t, "123", string(body))

	resp, err = app.Test(httptest.NewRequest("GET", "/get?cache=12345", nil))
	utils.AssertEqual(t, nil, err)
	body, err = ioutil.ReadAll(resp.Body)
	utils.AssertEqual(t, nil, err)
	utils.AssertEqual(t, "123", string(body))
}

func Test_Cache_NothingToCache(t *testing.T) {
	app := fiber.New()

	app.Use(New(Config{Expiration: -(time.Second * 1)}))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString(time.Now().String())
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	utils.AssertEqual(t, nil, err)
	body, err := ioutil.ReadAll(resp.Body)
	utils.AssertEqual(t, nil, err)

	time.Sleep(500 * time.Millisecond)

	respCached, err := app.Test(httptest.NewRequest("GET", "/", nil))
	utils.AssertEqual(t, nil, err)
	bodyCached, err := ioutil.ReadAll(respCached.Body)
	utils.AssertEqual(t, nil, err)

	if bytes.Equal(body, bodyCached) {
		t.Errorf("Cache should have expired: %s, %s", body, bodyCached)
	}
}

func Test_Cache_CustomNext(t *testing.T) {
	app := fiber.New()

	app.Use(New(Config{
		Next: func(c *fiber.Ctx) bool {
			return c.Response().StatusCode() != fiber.StatusOK
		},
		CacheControl: true,
	}))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString(time.Now().String())
	})

	app.Get("/error", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusInternalServerError).SendString(time.Now().String())
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	utils.AssertEqual(t, nil, err)
	body, err := ioutil.ReadAll(resp.Body)
	utils.AssertEqual(t, nil, err)

	respCached, err := app.Test(httptest.NewRequest("GET", "/", nil))
	utils.AssertEqual(t, nil, err)
	bodyCached, err := ioutil.ReadAll(respCached.Body)
	utils.AssertEqual(t, nil, err)
	utils.AssertEqual(t, true, bytes.Equal(body, bodyCached))
	utils.AssertEqual(t, true, respCached.Header.Get(fiber.HeaderCacheControl) != "")

	_, err = app.Test(httptest.NewRequest("GET", "/error", nil))
	utils.AssertEqual(t, nil, err)

	errRespCached, err := app.Test(httptest.NewRequest("GET", "/error", nil))
	utils.AssertEqual(t, nil, err)
	utils.AssertEqual(t, true, errRespCached.Header.Get(fiber.HeaderCacheControl) == "")
}

func Test_CustomKey(t *testing.T) {
	app := fiber.New()
	var called bool
	app.Use(New(Config{KeyGenerator: func(c *fiber.Ctx) string {
		called = true
		return c.Path()
	}}))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("hi")
	})

	req := httptest.NewRequest("GET", "/", nil)
	_, err := app.Test(req)
	utils.AssertEqual(t, nil, err)
	utils.AssertEqual(t, true, called)

}

func Test_CacheHeader(t *testing.T) {
	app := fiber.New()

	app.Use(New(Config{
		Expiration: 10 * time.Second,
		Next: func(c *fiber.Ctx) bool {
			return c.Response().StatusCode() != fiber.StatusOK
		},
	}))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Hello, World!")
	})

	app.Post("/", func(c *fiber.Ctx) error {
		return c.SendString(c.Query("cache"))
	})

	app.Get("/error", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusInternalServerError).SendString(time.Now().String())
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	utils.AssertEqual(t, nil, err)
	utils.AssertEqual(t, cacheMiss, resp.Header.Get("X-Cache"))

	resp, err = app.Test(httptest.NewRequest("GET", "/", nil))
	utils.AssertEqual(t, nil, err)
	utils.AssertEqual(t, cacheHit, resp.Header.Get("X-Cache"))

	resp, err = app.Test(httptest.NewRequest("POST", "/?cache=12345", nil))
	utils.AssertEqual(t, nil, err)
	utils.AssertEqual(t, cacheUnreachable, resp.Header.Get("X-Cache"))

	errRespCached, err := app.Test(httptest.NewRequest("GET", "/error", nil))
	utils.AssertEqual(t, nil, err)
	utils.AssertEqual(t, cacheUnreachable, errRespCached.Header.Get("X-Cache"))
}

func Test_CustomCacheHeader(t *testing.T) {
	app := fiber.New()

	app.Use(New(Config{
		CacheHeader: "Cache-Status",
	}))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Hello, World!")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	utils.AssertEqual(t, nil, err)
	utils.AssertEqual(t, cacheMiss, resp.Header.Get("Cache-Status"))
}

// go test -v -run=^$ -bench=Benchmark_Cache -benchmem -count=4
func Benchmark_Cache(b *testing.B) {
	app := fiber.New()

	app.Use(New())

	app.Get("/demo", func(c *fiber.Ctx) error {
		data, _ := ioutil.ReadFile("../../.github/README.md")
		return c.Status(fiber.StatusTeapot).Send(data)
	})

	h := app.Handler()

	fctx := &fasthttp.RequestCtx{}
	fctx.Request.Header.SetMethod("GET")
	fctx.Request.SetRequestURI("/demo")

	b.ReportAllocs()
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		h(fctx)
	}

	utils.AssertEqual(b, fiber.StatusTeapot, fctx.Response.Header.StatusCode())
	utils.AssertEqual(b, true, len(fctx.Response.Body()) > 30000)
}

// go test -v -run=^$ -bench=Benchmark_Cache_Storage -benchmem -count=4
func Benchmark_Cache_Storage(b *testing.B) {
	app := fiber.New()

	app.Use(New(Config{
		Storage: memory.New(),
	}))

	app.Get("/demo", func(c *fiber.Ctx) error {
		data, _ := ioutil.ReadFile("../../.github/README.md")
		return c.Status(fiber.StatusTeapot).Send(data)
	})

	h := app.Handler()

	fctx := &fasthttp.RequestCtx{}
	fctx.Request.Header.SetMethod("GET")
	fctx.Request.SetRequestURI("/demo")

	b.ReportAllocs()
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		h(fctx)
	}

	utils.AssertEqual(b, fiber.StatusTeapot, fctx.Response.Header.StatusCode())
	utils.AssertEqual(b, true, len(fctx.Response.Body()) > 30000)
}
