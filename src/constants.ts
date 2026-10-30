export const OFFERS_VIEW_TYPE = 'sbe-offers-view';
export const OFFERS_APP_ID = 'offers';
export const OFFERS_PLUGIN_ID = 'sbe-offers';

/**
 * Ключи `form_values`, управляющие показом в документе (Task 11, 2026-10-30).
 *
 * Повторяют константы сервера `formFlagBriefSummary` / `formFlagSectionCopy`
 * в `offers-service/docx.go`: это контракт JSON, и разойтись они не должны.
 * Флаг включён значением «1» (сервер понимает и true/yes/да).
 */
export const FORM_FLAG_BRIEF_SUMMARY = 'doc.brief_summary';
export const FORM_FLAG_SECTION_COPY = 'doc.section_copy';