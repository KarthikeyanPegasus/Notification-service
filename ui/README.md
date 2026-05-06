## E2E tests (Playwright)

The UI ships with Playwright end-to-end tests in `ui/tests/`.

- **Run tests**

```bash
npx playwright install --with-deps chromium
npm run test:e2e
```

- **Open Playwright UI**

```bash
npm run test:e2e:ui
```

### Notes

- Tests start the Next.js server automatically via `playwright.config.ts`.
- Clerk is disabled during E2E runs (`NEXT_PUBLIC_CLERK_ENABLED=false`) so the app uses the built-in mock auth provider.
- Backend calls to `/v1/*` are mocked in tests (see `tests/utils/mock-api.ts`) to keep E2E runs stable without requiring the API service.

