import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

// Приложение раздаётся Caddy по пути /app-offers/ (см. §9.1 плана миграции).
// base: './' — ссылки на ассеты относительные, поэтому один и тот же dist/
// работает и на сервере по пути, и локально при `vite preview`.
//
// Маршрутизация — по хэшу (#/offer/123): серверу не нужно переписывать
// несуществующие пути на index.html, а боту достаточно передать url целиком.
export default defineConfig({
  base: './',
  plugins: [react()],
  build: {
    target: 'es2020',
    sourcemap: false,
    reportCompressedSize: true,
    // Бюджет плана — до 200 КБ gzip (START-HERE §5.4). Предупреждение Vite
    // считает НЕсжатый размер, поэтому порог выше: gzip-бюджет проверяется
    // отдельно командой `npm run size` (scripts/check_size.mjs).
    chunkSizeWarningLimit: 400,
  },
});