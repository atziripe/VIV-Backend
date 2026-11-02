// k6 load test against a VIV Backend deployment (dev by default — never
// point this at production without the team's explicit go-ahead, since
// it generates real traffic and real Neon/OpenAI usage).
//
// Every protected route needs a real Firebase ID token (see
// FirebaseAuthMiddleware), so setup() signs in once via the Firebase Auth
// REST API and reuses that token for the whole run — fine for a run under
// ~55 minutes (ID tokens last 1 hour); for a longer soak test, refresh
// the token periodically instead.
//
// Required env vars:
//   BASE_URL              e.g. https://viv-backend-dev.up.railway.app
//   FIREBASE_WEB_API_KEY  the Web API key from Firebase project settings
//                         (the same one the mobile app already ships with —
//                         not secret, safe to pass on the command line)
//   TEST_USER_EMAIL       a real test account's email in that Firebase project
//   TEST_USER_PASSWORD    that account's password
//
// Run:
//   k6 run \
//     -e BASE_URL=https://your-dev-url \
//     -e FIREBASE_WEB_API_KEY=... \
//     -e TEST_USER_EMAIL=loadtest@example.com \
//     -e TEST_USER_PASSWORD=... \
//     loadtest/smoke.js
//
// Tune concurrency by editing the `stages` below, or override on the
// command line with -e VUS=50 -e DURATION=2m for a flat, no-ramp run.

import http from "k6/http";
import { check, group, sleep } from "k6";
import { Rate, Trend } from "k6/metrics";

const BASE_URL = __ENV.BASE_URL;
const FIREBASE_WEB_API_KEY = __ENV.FIREBASE_WEB_API_KEY;
const TEST_USER_EMAIL = __ENV.TEST_USER_EMAIL;
const TEST_USER_PASSWORD = __ENV.TEST_USER_PASSWORD;

const errorRate = new Rate("viv_errors");
const meTrend = new Trend("viv_me_duration");
const homeTrend = new Trend("viv_home_duration");
const plansTrend = new Trend("viv_plans_duration");
const nutritionTrend = new Trend("viv_nutrition_duration");

export const options = __ENV.VUS
  ? {
      vus: Number(__ENV.VUS),
      duration: __ENV.DURATION || "1m",
      thresholds: {
        http_req_duration: ["p(95)<1500"],
        viv_errors: ["rate<0.01"],
      },
    }
  : {
      // Default: ramp 0 -> 50 concurrent VUs over 2 steps, hold, ramp down.
      // Start here; raise the plateau once you know where it breaks.
      stages: [
        { duration: "30s", target: 10 },
        { duration: "1m", target: 25 },
        { duration: "2m", target: 50 },
        { duration: "30s", target: 0 },
      ],
      thresholds: {
        http_req_duration: ["p(95)<1500"],
        viv_errors: ["rate<0.01"],
      },
    };

export function setup() {
  if (!BASE_URL || !FIREBASE_WEB_API_KEY || !TEST_USER_EMAIL || !TEST_USER_PASSWORD) {
    throw new Error(
      "Missing required env vars — need BASE_URL, FIREBASE_WEB_API_KEY, TEST_USER_EMAIL, TEST_USER_PASSWORD"
    );
  }

  const signInRes = http.post(
    `https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=${FIREBASE_WEB_API_KEY}`,
    JSON.stringify({
      email: TEST_USER_EMAIL,
      password: TEST_USER_PASSWORD,
      returnSecureToken: true,
    }),
    { headers: { "Content-Type": "application/json" } }
  );

  if (signInRes.status !== 200) {
    throw new Error(`Firebase sign-in failed: ${signInRes.status} ${signInRes.body}`);
  }

  const idToken = signInRes.json("idToken");
  return { idToken };
}

export default function (data) {
  const headers = {
    Authorization: `Bearer ${data.idToken}`,
    "Content-Type": "application/json",
  };

  // Mix of read-only, representative endpoints — no mutations, nothing
  // that kicks off an LLM generation job, so this is safe to run
  // repeatedly without polluting data or burning OpenAI quota.
  group("me", () => {
    const res = http.get(`${BASE_URL}/me`, { headers, tags: { name: "GetMe" } });
    meTrend.add(res.timings.duration);
    errorRate.add(res.status >= 500);
    check(res, { "GET /me: status < 500": (r) => r.status < 500 });
  });

  const today = new Date().toISOString().slice(0, 10);
  group("home", () => {
    const res = http.get(`${BASE_URL}/home/today?date=${today}`, {
      headers,
      tags: { name: "GetHomeToday" },
    });
    homeTrend.add(res.timings.duration);
    errorRate.add(res.status >= 500);
    check(res, { "GET /home/today: status < 500": (r) => r.status < 500 });
  });

  group("plans", () => {
    const res = http.get(`${BASE_URL}/plans/current`, { headers, tags: { name: "GetCurrentPlan" } });
    plansTrend.add(res.timings.duration);
    errorRate.add(res.status >= 500);
    check(res, { "GET /plans/current: status < 500": (r) => r.status < 500 });
  });

  group("nutrition", () => {
    const res = http.get(`${BASE_URL}/nutrition/plan`, { headers, tags: { name: "GetNutritionPlan" } });
    nutritionTrend.add(res.timings.duration);
    errorRate.add(res.status >= 500);
    check(res, { "GET /nutrition/plan: status < 500": (r) => r.status < 500 });
  });

  sleep(1); // rough "a real user pauses between taps" — delete to hammer harder
}
