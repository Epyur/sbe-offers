package main

import (
	"context"
	"log"
	"net/http"
	"time"
)

// Журнал генерации и честные таймауты (2026-11-06, шаг A плана
// «генерация без модели»).
//
// Повод: администратор поменял структуру промтов в макете, и система «ушла
// думать и не вернулась» — ни ошибки, ни сообщения. Замерено: модели разрешено
// 180 с на попытку (три попытки), HTTP `WriteTimeout` стоял 90 с, бот ждал 60 с,
// плагин — бесконечно. Длинный ответ не мог дойти до человека В ПРИНЦИПЕ.
//
// Здесь две вещи, и обе — про «видно, что происходит»:
//  1. `WriteTimeout` сервера снят, у каждой долгой ручки свой предел
//     (`writeTimeoutFor`), потому что 90 с на всех делило длинный документ
//     ровно пополам;
//  2. текст отказа модели и успех сборки попадают в журнал с продолжительностью
//     и числом вызовов модели — по одной строке на вызов и одна сводка.

// Пределы ожидания по видам работы, в секундах.
//
// Значения выбраны от худшего законного ответа модели, а не «покруглее»:
// текст на 4000 знаков модель пишет 58 с (замер 2026-11-06), попыток три с
// паузами — около 12 минут на самый плохой случай. Меньше ставить нельзя:
// именно это и обрезало ответ.
const (
	// generationSinkTimeout — запись ответа ручки в сеть.
	generationSinkTimeout = 600 * time.Second
	// layoutBuildTimeout — предложение по загруженному макету.
	layoutBuildTimeout = 600 * time.Second
	// systemTextTimeout — тексты системы при её выборе (два блока).
	systemTextTimeout = 420 * time.Second
	// quickResponseTimeout — обычные ручки: без модели и без долгих обходов.
	quickResponseTimeout = 60 * time.Second
	// blockTextPersistTimeout — запись уже написанных текстов в КП.
	//
	// Свой короткий предел на ОТДЕЛЬНЫЙ контекст без отмены запроса (2026-11-14):
	// модель уже ответила, и обрыв соединения не должен выбрасывать
	// написанное (живое КП 031/2026 перегенерировалось на каждой сборке).
	blockTextPersistTimeout = 30 * time.Second
)

// writeTimeoutFor — сколько запросу отведено на ответ.
//
// Считается по пути и методу, а не задаётся флагом `http.Server`: одно число
// на сервер и было причиной тишины в боте. Сборка документа ждёт модель (до
// нескольких минут), ручка списка — нет, и общий предел обязан быть разным.
func writeTimeoutFor(path string) time.Duration {
	switch {
	case hasSuffixPath(path, "/docx"), hasSuffixPath(path, "/link"),
		hasSuffixPath(path, "/export"), hasSuffixPath(path, "/app/answer"):
		// `/app/answer` — «Отправить в чат» из Mini App (2026-11-14): собирает тот же
		// документ, что и «⬇ DOCX». Без этой строки ручка получала 60 с: макет
		// не успевал (уходил в блочный путь), а тексты систем обрывались
		// на второй системе (живое КП 031/2026).
		return layoutBuildTimeout
	case hasSuffixPath(path, "/system"), hasSuffixPath(path, "/recalculate"),
		hasSuffixPath(path, "/material"):
		return systemTextTimeout
	}
	return quickResponseTimeout
}

// hasSuffixPath — путь кончается на хвост (без учёта query).
func hasSuffixPath(path, suffix string) bool {
	if query := indexByte(path, '?'); query >= 0 {
		path = path[:query]
	}
	if len(path) < len(suffix) {
		return false
	}
	return path[len(path)-len(suffix):] == suffix
}

// indexByte — первое вхождение байта; -1, если нет.
//
// Своя вместо strings.Index ради того, чтобы не тянуть строковый пакет в
// файл, где всё остальное — про сеть и время; поведение то же.
func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// generationLog — один вызов модели: что просили и чем кончилось.
//
// Пишется ДО ответа (started) и после (done), потому что «модель думала и
// молчала» опознаётся именно по отсутствию второй строки: видно, что запрос
// ушёл и не вернулся. Содержимое промпта и ответа НЕ пишется — в них данные
// заказчика (правило проекта: отказ по содержимому и отказ по доступу — два
// разных журнала).
func logModelCall(kind, key string, instructionLen int, started time.Time, err error) {
	elapsed := time.Since(started).Round(time.Millisecond)
	status := "ok"
	if err != nil {
		status = "error: " + err.Error()
	}
	log.Printf("gen: %s key=%s instr=%d %s %s", kind, key, instructionLen, elapsed, status)
}

// buildReport — итог одной сборки документа: сколько текстов взято из кэша,
// сколько написано моделью, сколько не получилось.
//
// Сводка в журнале — единственный способ ответить человеку «почему документ
// пустой» после факта: по одной строке на вызов видно, какой именно текст
// молчал.
type buildReport struct {
	FromCache   int
	Generated   int
	Failed      int
	Started     time.Time
	offerNumber string
}

// newBuildReport — начало сборки для журнала.
func newBuildReport(offerNumber string) *buildReport {
	return &buildReport{Started: time.Now(), offerNumber: offerNumber}
}

// logSummary пишет одну строку на сборку. Вызывается из defer, чтобы сводка
// была и при ошибке сборки.
func (r *buildReport) logSummary() {
	if r == nil {
		return
	}
	log.Printf("gen: build offer=%s cached=%d generated=%d failed=%d %s",
		r.offerNumber, r.FromCache, r.Generated, r.Failed,
		time.Since(r.Started).Round(time.Millisecond))
}

// withWorkTimeout даёт запросу контекст с пределом по виду работы.
//
// `http.Server.WriteTimeout` снят, поэтому единственный предел — этот: обработчик
// получает контекст, а модель и БД считаются с ним же. Отмена доходит до
// HTTP-клиента модели, и запрос отпускается, а не висит до перезапуска.
func withWorkTimeout(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), writeTimeoutFor(r.URL.Path))
}
