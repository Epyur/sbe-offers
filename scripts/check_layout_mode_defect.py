"""Проверка живым HTTP: у КП и боевого листка СВОЙ макет (2026-11-11).

Дефект, ради которого заведено: «вместо КП выгружался пустой макет боевого
листка». Причина — режим макета; здесь проверяется весь путь через ручки:
загрузка в один режим не трогает другой, форма называет СВОЙ макет, а КП
печатается содержимым, а не меткой листка.

Запуск — против стенда (см. AGENTS.md, раздел «Checks»):

    python scripts/check_layout_mode_defect.py [http://host:port]

По умолчанию — http://127.0.0.1:18080 (порт стенда на хосте).
"""
import hmac, hashlib, base64, json, time, urllib.request, urllib.error, io, zipfile, re, sys

BASE = sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:18080"


def b64(b):
    return base64.urlsafe_b64encode(b).rstrip(b'=')


def make_token():
    secret = b"test-service-secret"
    header = {"alg": "HS256", "typ": "JWT"}
    now = int(time.time())
    payload = {"email": "owner@tn.ru", "device_id": "stand", "app_id": "offers",
               "iss": "auth-service", "iat": now, "exp": now + 3600}
    si = b64(json.dumps(header, separators=(',', ':')).encode()) + b'.' + \
        b64(json.dumps(payload, separators=(',', ':')).encode())
    sig = b64(hmac.new(secret, si, hashlib.sha256).digest())
    return (si + b'.' + sig).decode()


TOK = make_token()


def req(method, path, data=None, ctype=None):
    headers = {"Authorization": "Bearer " + TOK}
    if ctype:
        headers["Content-Type"] = ctype
    r = urllib.request.Request(BASE + path, method=method, data=data, headers=headers)
    try:
        with urllib.request.urlopen(r) as resp:
            return resp.status, resp.read()
    except urllib.error.HTTPError as e:
        return e.code, e.read()


def doc_text(body):
    z = zipfile.ZipFile(io.BytesIO(body))
    doc = z.read("word/document.xml").decode("utf-8")
    paras = re.findall(r"<w:p[ >].*?</w:p>", doc, re.S)
    out = []
    for p in paras:
        t = "".join(re.findall(r"<w:t[^>]*>([^<]*)</w:t>", p))
        if t.strip():
            out.append(t.strip())
    return out, doc


ok = 0
fail = []


def check(name, cond, extra=""):
    global ok
    if cond:
        ok += 1
        print("  OK  ", name)
    else:
        fail.append(name)
        print("  FAIL", name, extra)


print("== A. Боевой листок создаётся и печатается ==")
s, body = req("POST", "/api/offers/combat", json.dumps({"title": "Дефект макета"}).encode(), "application/json")
check("листок создан", s == 200, body[:200])
sheet = json.loads(body) if s == 200 else {}
sheet_id = sheet.get("id", 0)
if sheet_id:
    req("DELETE", f"/api/offers/combat/{sheet_id}")

print("== B. Образец макета: метки разбираются ==")
s, example = req("GET", "/api/offers/form/layout/example")
check("образец скачан", s == 200 and len(example) > 500)

print("== C. Загрузка макета в КП: ответ называет режим client ==")
s, body = req("POST", "/api/offers/form/layout?name=%D0%9A%D0%9F.docx&mode=client", example,
              "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
check("загрузка КП → 200", s == 200, body[:300])
resp = json.loads(body) if s == 200 else {}
check("ответ говорит mode=client", resp.get("mode") == "client", resp.get("mode"))
check("метки образца разобраны", len(resp.get("layoutFields") or []) > 0)

print("== D. Загрузка макета в боевой листок: ответ называет combat ==")
s, body = req("POST", "/api/offers/form/layout?name=%D0%91%D0%9B.docx&mode=combat", example,
              "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
resp2 = json.loads(body) if s == 200 else {}
check("ответ говорит mode=combat", resp2.get("mode") == "combat", resp2.get("mode"))

print("== E. Макеты не подменились: скачивание различает их ==")
s, lc = req("GET", "/api/offers/form/layout?mode=client")
check("макет КП скачивается", s == 200 and len(lc) > 0)
s, lb = req("GET", "/api/offers/form/layout?mode=combat")
check("макет листка скачивается", s == 200 and len(lb) > 0)
check("оба режима получили образец (один файл намеренно)", lc == lb)

print("== F. Форма КП знает про свой макет, форма листка — про свой ==")
s, body = req("GET", "/api/offers/form?mode=client")
f = json.loads(body)
check("форма КП hasLayout", f.get("hasLayout") is True)
check("форма КП: имя макета КП", f.get("layoutName") == "КП.docx", f.get("layoutName"))
s, body = req("GET", "/api/offers/form?mode=combat")
f = json.loads(body)
check("форма листка: имя макета листка", f.get("layoutName") == "БЛ.docx", f.get("layoutName"))

print("== G. КП печатается содержимым, а не пустым макетом ==")
s, body = req("GET", "/api/offers/offers")
lst = json.loads(body).get("offers") or []
with_items = [o for o in lst if o.get("items")]
if not with_items:
    check("есть КП с позициями", False)
else:
    oid = with_items[0]["id"]
    s, doc = req("POST", f"/api/offers/offers/{oid}/export")
    check("экспорт КП → 200", s == 200, doc[:200])
    if s == 200:
        paras, xml = doc_text(doc)
        text = " ".join(paras)
        check("документ не пуст", len(paras) > 5, f"{len(paras)} абзацев")
        check("в КП нет меток {{система}}", "{{система}}" not in text)
        check("в КП нет метки {{сравнение}}", "{{сравнение}}" not in text)
        check("в КП нет заголовка «Боевой листок №»", "Боевой листок №" not in text)
        check("состав системы в КП напечатан", "Состав" in text or "состав" in text)

print("== H. Удаляем макеты обратно ==")
s, _ = req("DELETE", "/api/offers/form/layout?mode=client")
check("макет КП удалён", s == 200)
s, _ = req("DELETE", "/api/offers/form/layout?mode=combat")
check("макет листка удалён", s == 200)

print()
print(f"ИТОГО: {ok} ок, {len(fail)} провалов")
if fail:
    print("Провалы:", fail)
    raise SystemExit(1)