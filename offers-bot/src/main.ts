import { startVerifyServer } from './verify-server.ts';
import { runPoller } from './poller.ts';

// Проверка токена из настроек плагина и приём сообщений — два разных потока.
// Проверка не должна ждать поллинга, а поллинг не должен падать из-за её проблем.
startVerifyServer();
void runPoller();