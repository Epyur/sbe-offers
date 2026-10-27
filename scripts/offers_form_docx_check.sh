#!/bin/sh
# Проверка на сервере: сохранённая форма КП доходит до DOCX.
# Выполняется НА СЕРВЕРЕ (sh /tmp/offers-deploy/offers_form_docx_check.sh).
set -e
cd /opt/mailers
. ./.env

BASE=https://epyur.fvds.ru/api/offers
TOKEN=$(docker compose exec -T offers wget -qO- \
  --header="Content-Type: application/json" \
  --header="X-Service-Secret: $OFFERS_SERVICE_SECRET" \
  --post-data='{"target_app_id":"offers"}' \
  http://auth-service:3000/internal/service-token \
  | sed -n 's/.*"jwt":"\([^"]*\)".*/\1/p')

STAMP=$(date +%s)
TITLE="ПРОВЕРКА ФОРМЫ $STAMP"

echo "== сохраняем форму =="
cat > /tmp/form.json <<JSON
{"mode":"client","title":"$TITLE",
 "placeholders":[{"id":"client_name","label":"Клиент"},{"id":"valid_until","label":"До"}],
 "blocks":[{"id":"intro","title":"Вступление",
   "content":[{"type":"paragraph","text":"Уважаемый(ая) {{client_name}}!"}],
   "showInClient":true,"showInInternal":false}],
 "requiredFields":["client_name"]}
JSON
SAVED=$(curl -s -X PUT "$BASE/form" -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" --data-binary @/tmp/form.json)
echo "$SAVED" | grep -q "$TITLE" && echo "  ✓ форма сохраняется и возвращается как есть" || echo "  ✗ форма вернулась не та: $SAVED"

echo "== создаём КП =="
OFFER=$(curl -s -X POST "$BASE/offers" -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"client":{"name":"Проверка","contact":"Тест"},"items":[{"name":"Позиция","unit":"шт","qty":1,"price":100}],"formValues":{"client_name":"Проверка","valid_until":"01.12.2026"}}')
OFFER_ID=$(echo "$OFFER" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
echo "  предложение id=$OFFER_ID"

echo "== DOCX =="
curl -s -o /tmp/form-check.docx -X POST "$BASE/offers/$OFFER_ID/export?format=docx" \
  -H "Authorization: Bearer $TOKEN"
rm -rf /tmp/form-check && mkdir -p /tmp/form-check
cd /tmp/form-check && unzip -q /tmp/form-check.docx
if grep -q "$TITLE" word/document.xml; then
  echo "  ✓ заголовок сохранённой формы попал в DOCX"
  OK=1
else
  echo "  ✗ заголовок формы в DOCX не найден"
  grep -o '<w:t[^>]*>[^<]*</w:t>' word/document.xml | head -5
  OK=0
fi
if grep -q 'Уважаемый(ая) Проверка!' word/document.xml; then
  echo "  ✓ блок формы с подстановкой поля в документе"
else
  echo "  ✗ блок формы не попал в документ"
  OK=0
fi

echo "== убираем тестовые данные и возвращаем форму по умолчанию =="
cd /opt/mailers
curl -s -o /dev/null -X DELETE "$BASE/offers/$OFFER_ID" -H "Authorization: Bearer $TOKEN"
cat > /tmp/form-default.json <<'JSON'
{"mode":"client","title":"Коммерческое предложение",
 "placeholders":[{"id":"client_name","label":"Наименование клиента"},
                 {"id":"client_contact","label":"Контактное лицо"},
                 {"id":"valid_until","label":"Предложение действительно до"},
                 {"id":"conditions","label":"Условия"}],
 "blocks":[{"id":"intro","title":"Вступление","content":[{"type":"paragraph","text":"Предлагаем Вам поставку материалов и выполнение работ по следующим позициям."}],"showInClient":true,"showInInternal":false},
           {"id":"conditions","title":"Условия","content":[{"type":"paragraph","text":"Условия: {{conditions}}"}],"showInClient":true,"showInInternal":false}],
 "requiredFields":["client_name"]}
JSON
curl -s -o /dev/null -X PUT "$BASE/form" -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" --data-binary @/tmp/form-default.json
echo "  готово"

exit $((1 - OK))