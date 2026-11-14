import { test, expect } from "@playwright/test";
test("frontend proxy preserves the browser origin for protected mutations", async ({
  page,
}) => {
  await page.goto("/app/");
  const result = await page.evaluate(async () => {
    const response = await fetch("/ui-api/contract", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Workbench-Request": "1",
      },
      body: JSON.stringify({ synthetic: true }),
    });
    return { status: response.status, value: await response.json() };
  });
  expect(result.status).toBe(200);
  expect(result.value).toMatchObject({
    method: "POST",
    host: "127.0.0.1:4173",
    origin: "http://127.0.0.1:4173",
    body: { synthetic: true },
    authorization: null,
  });
});
test("the synthetic ingress still rejects a foreign origin", async ({
  request,
}) => {
  const response = await request.post("/ui-api/contract", {
    headers: { Origin: "http://foreign.example" },
    data: { synthetic: true },
  });
  expect(response.status()).toBe(403);
  expect(await response.json()).toEqual({ code: "ORIGIN_REJECTED" });
});
