import { useSettingsStore } from './store/settingsStore'
import SettingsLayout from './components/settings/SettingsLayout'
import DashboardPage from './pages/settings/DashboardPage'
import ObservabilityPage from './pages/settings/ObservabilityPage'
import ChatPage from './pages/settings/ChatPage'
import DiaryPage from './pages/settings/DiaryPage'
import MemoryIdentityPage from './pages/settings/MemoryIdentityPage'
import StrategyLabPage from './pages/settings/StrategyLabPage'
import ApiConfigPage from './pages/settings/ApiConfigPage'
import PluginsPage from './pages/settings/PluginsPage'
import PluginConfigPage from './pages/settings/PluginConfigPage'

const pages: Record<string, React.ComponentType> = {
  dashboard: DashboardPage,
  observability: ObservabilityPage,
  chat: ChatPage,
  diary: DiaryPage,
  memory: MemoryIdentityPage,
  'strategy-lab': StrategyLabPage,
  api: ApiConfigPage,
  plugins: PluginsPage,
  'plugin-config': PluginConfigPage,
}

function SettingsApp() {
  const currentPage = useSettingsStore((s) => s.currentPage)
  const Page = pages[currentPage] || DashboardPage
  return <SettingsLayout><Page /></SettingsLayout>
}

export default SettingsApp
