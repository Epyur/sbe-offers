package main

// Печать боевого листка без формы (2026-11-16, шаг 2a).
//
// Печать стала методом сервера: разделы листка читаются из формы
// (`s.loadForm`), и второго пути «печать без сервера» заводить не нужно.
// Тестам же нужен листок без базы — печать по старому виду (карточки).
// `combatNoForm` отдаёт такой сервер: `pool` пуст, `loadForm` отвечает ошибкой,
// и печать честно уходит в запасной вид. Это не подделка поведения, а ровно
// то, что делает живой сервис, когда формы листка ещё нет.

import "context"

// combatRenderFunc — подпись печати листка (метод `*Server`).
type combatRenderFunc func(*Server, context.Context, []byte, CombatSheet, []SystemCard, []SystemCard, ImageSource) ([]byte, error)

// combatNoForm — печать листка без формы: разделы не читаются, печатается
// карточное сравнение (тот же путь, что «разделов нет»).
func combatNoForm(render combatRenderFunc) func(context.Context, []byte, CombatSheet, []SystemCard, []SystemCard, ImageSource) ([]byte, error) {
	return func(ctx context.Context, layout []byte, sheet CombatSheet, our, competitor []SystemCard, images ImageSource) ([]byte, error) {
		return render(&Server{}, ctx, layout, sheet, our, competitor, images)
	}
}
