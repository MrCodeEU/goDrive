import { expect, test, type Page } from "@playwright/test";

async function login(page: Page) {
  await page.goto("/");
  await page.getByLabel("Username").fill("admin");
  await page.getByLabel("Password").fill("change-me-e2e");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.locator(".sidebar").getByRole("button", { name: "My files" })).toBeVisible();
  await expect(page.getByRole("button", { name: "New folder" })).toBeVisible();
}

function fileCard(page: Page, name: string) {
  return page.locator(".file-card", { hasText: name }).first();
}

test("desktop file workflow covers login, browse, upload, search, preview, trash, admin, and logout", async ({ page }) => {
  const uploadedName = `e2e-note-${Date.now()}.txt`;
  const folderName = `E2E Folder ${Date.now()}`;

  await login(page);

  await page.getByRole("button", { name: "New folder" }).click();
  await page.locator(".action-panel").getByLabel("Name").fill(folderName);
  await page.locator(".action-panel").getByRole("button", { name: "Apply" }).click();
  await expect(fileCard(page, folderName)).toBeVisible();

  await fileCard(page, folderName).dblclick();
  await expect(page.getByRole("button", { name: "Back to parent folder" })).toBeVisible();

  await page.locator('input[type="file"]').setInputFiles({
    name: uploadedName,
    mimeType: "text/plain",
    buffer: Buffer.from("Hello from Playwright e2e.\nThis verifies text preview.\n")
  });
  await expect(fileCard(page, uploadedName)).toBeVisible();
  await expect(page.locator(".upload-queue")).toContainText("done", { timeout: 15_000 });

  await fileCard(page, uploadedName).dblclick();
  const viewer = page.getByRole("dialog", { name: uploadedName });
  await expect(viewer).toBeVisible();
  await expect(viewer.getByText("Hello from Playwright e2e.")).toBeVisible();
  await viewer.getByRole("button", { name: "Back" }).click();

  await fileCard(page, uploadedName).click();
  await page.getByRole("button", { name: "Info" }).click();
  const infoPanel = page.locator(".info-panel");
  await expect(infoPanel.getByRole("heading", { name: "File info" })).toBeVisible();
  await expect(infoPanel.locator(".info-row").first()).toContainText(uploadedName);
  await infoPanel.getByRole("button", { name: "×" }).click();

  await page.getByPlaceholder("Search files…").first().fill(uploadedName);
  await expect(page.locator(".search-result", { hasText: uploadedName })).toBeVisible();
  await page.keyboard.press("Escape");

  await fileCard(page, uploadedName).click();
  await page.getByRole("button", { name: "Delete" }).click();
  await expect(page.getByRole("heading", { name: "Move to trash" })).toBeVisible();
  await page.getByRole("button", { name: "Move to trash" }).click();
  await expect(fileCard(page, uploadedName)).toHaveCount(0);

  await page.getByRole("button", { name: "Trash" }).click();
  await expect(page.getByRole("heading", { name: "Trash" })).toBeVisible();
  await expect(page.locator(".trash-item", { hasText: uploadedName })).toBeVisible();
  await page.locator(".trash-item", { hasText: uploadedName }).getByRole("button", { name: "Restore" }).click();
  await expect(fileCard(page, uploadedName)).toBeVisible();
  await page.locator(".trash-panel").getByRole("button", { name: "×" }).click();
  await expect(fileCard(page, uploadedName)).toBeVisible();

  await page.getByTitle("Admin").click();
  await expect(page.getByRole("heading", { name: "Admin" })).toBeVisible();
  await expect(page.getByText("Indexed")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Users" })).toBeVisible();
  await page.getByRole("heading", { name: "Admin" }).locator("..").getByRole("button", { name: "×" }).click();

  await page.getByTitle("Logout").click();
  await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible();
});

test("mobile layout keeps core navigation usable", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await login(page);

  await page.getByRole("button", { name: "Open navigation" }).click();
  await expect(page.locator(".sidebar.mobile-open")).toBeVisible();
  await page.locator(".sidebar.mobile-open").getByRole("button", { name: "My files" }).click();
  await expect(page.locator(".sidebar.mobile-open")).toHaveCount(0);

  await expect(page.getByRole("button", { name: "New folder" })).toBeVisible();
  await expect(page.getByPlaceholder("Search files…").first()).toBeVisible();
  await page.getByTitle("Admin").click();
  await expect(page.getByRole("heading", { name: "Admin" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Users" })).toBeVisible();
});

test("tablet layout keeps browse, preview, trash, and admin usable", async ({ page }) => {
  await page.setViewportSize({ width: 768, height: 1024 });
  const uploadedName = `tablet-note-${Date.now()}.txt`;

  await login(page);

  await expect(page.getByRole("button", { name: "Open navigation" })).toHaveCount(0);
  await expect(page.locator(".sidebar")).toBeVisible();
  await expect(page.getByPlaceholder("Search files…").first()).toBeVisible();
  await expect(page.getByRole("button", { name: "New folder" })).toBeVisible();
  await expect(page.getByTitle("Grid view")).toBeVisible();
  await expect(page.getByTitle("List view")).toBeVisible();

  await page.locator('input[type="file"]').setInputFiles({
    name: uploadedName,
    mimeType: "text/plain",
    buffer: Buffer.from("Tablet viewport preview fixture.\n")
  });
  await expect(fileCard(page, uploadedName)).toBeVisible();

  await fileCard(page, uploadedName).dblclick();
  const viewer = page.getByRole("dialog", { name: uploadedName });
  await expect(viewer).toBeVisible();
  await expect(viewer.getByText("Tablet viewport preview fixture.")).toBeVisible();
  await viewer.getByRole("button", { name: "Back" }).click();

  await page.getByRole("button", { name: "Trash" }).click();
  await expect(page.locator(".trash-panel")).toBeVisible();
  await page.locator(".trash-panel").getByRole("button", { name: "×" }).click();

  await page.getByTitle("Admin").click();
  await expect(page.locator(".admin-panel")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Users" })).toBeVisible();
});
