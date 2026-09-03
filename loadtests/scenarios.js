// k6 load-test для marketplace.
//
// Запуск локально:
//   docker compose -f docker-compose.yml -f docker-compose.test.yml up -d --wait
//   docker compose --profile loadtest run --rm k6 run /scripts/scenarios.js \
//       --summary-export=/scripts/summary.json
//
// VUs / duration / thresholds выставлены по требованиям задания (минимум 10 VU,
// минимум 30 секунд) с лёгким запасом. Подробности — в плане.
import http from "k6/http";
import { check, sleep } from "k6";

const MARKETPLACE_URL = __ENV.MARKETPLACE_URL || "http://marketplace-api:8080";
const USER_SERVICE_URL = __ENV.USER_SERVICE_URL || "http://user-service:8000";

export const options = {
    vus: 10,
    duration: "60s",
    thresholds: {
        // p95 запросов меньше 500ms.
        http_req_duration: ["p(95)<500"],
        // Доля отказов меньше 1%.
        http_req_failed: ["rate<0.01"],
        // Отдельный порог на checks (логические assertions внутри сценария).
        checks: ["rate>0.99"],
    },
};

function registerUser(email, role) {
    const res = http.post(
        `${USER_SERVICE_URL}/auth/register`,
        JSON.stringify({ email, password: "secret123", role }),
        { headers: { "Content-Type": "application/json" } }
    );
    if (res.status !== 201) {
        throw new Error(`register ${email}: status=${res.status} body=${res.body}`);
    }
    return res.json();
}

function createProduct(token, name) {
    const res = http.post(
        `${MARKETPLACE_URL}/products`,
        JSON.stringify({
            name,
            price: 100.0,
            // Большой stock, чтобы хватило на все ордера в тесте.
            stock: 10000,
            category: "loadtest",
        }),
        {
            headers: {
                "Content-Type": "application/json",
                Authorization: `Bearer ${token}`,
            },
        }
    );
    if (res.status !== 201) {
        throw new Error(`create product: status=${res.status} body=${res.body}`);
    }
    return res.json();
}

// setup() выполняется один раз до старта VU. Создаём по SELLER+USER+product на
// каждый VU, чтобы потом не упираться в HasActiveOrder constraint при выписке
// заказов (каждый VU работает в изолированном контексте).
export function setup() {
    const sellers = [];
    const users = [];
    const products = [];

    for (let i = 0; i < options.vus; i++) {
        const sellerTag = `seller-${__ENV.K6_SEED || ""}${i}-${Date.now()}@loadtest`;
        const userTag = `user-${__ENV.K6_SEED || ""}${i}-${Date.now()}@loadtest`;

        const seller = registerUser(sellerTag, "SELLER");
        const product = createProduct(seller.access_token, `LoadProduct-${i}`);
        const user = registerUser(userTag, "USER");

        sellers.push(seller);
        users.push(user);
        products.push(product);
    }

    return { users, products };
}

export default function (data) {
    const i = (__VU - 1) % data.users.length;
    const userToken = data.users[i].access_token;
    const productId = data.products[i].id;

    const headers = {
        "Content-Type": "application/json",
        Authorization: `Bearer ${userToken}`,
    };

    const roll = Math.random();
    if (roll < 0.7) {
        // GET /products — публичный список.
        const r = http.get(`${MARKETPLACE_URL}/products`);
        check(r, { "list products 200": (r) => r.status === 200 });
    } else if (roll < 0.9) {
        // GET /products/{id} — конкретный продукт.
        const r = http.get(`${MARKETPLACE_URL}/products/${productId}`);
        check(r, { "get product 200": (r) => r.status === 200 });
    } else {
        // POST /orders + cancel — оформление заказа в транзакции, освобождаем
        // HasActiveOrder сразу после создания, чтобы следующая итерация VU
        // не упёрлась в "active order exists".
        const orderRes = http.post(
            `${MARKETPLACE_URL}/orders`,
            JSON.stringify({ items: [{ product_id: productId, quantity: 1 }] }),
            { headers }
        );
        const ok = check(orderRes, {
            "create order 201": (r) => r.status === 201,
        });
        if (ok && orderRes.status === 201) {
            const orderId = orderRes.json("id");
            const cancelRes = http.post(
                `${MARKETPLACE_URL}/orders/${orderId}/cancel`,
                null,
                { headers }
            );
            check(cancelRes, {
                "cancel order 200": (r) => r.status === 200,
            });
        }
    }

    sleep(0.2);
}
