import { BrowserRouter, Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import DashboardPage from './pages/Dashboard'
import SitesPage from './pages/Sites'
import RuntimesPage from './pages/Runtimes'
import ConfigPage from './pages/Config'
import LogsPage from './pages/Logs'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<DashboardPage />} />
          <Route path="/sites" element={<SitesPage />} />
          <Route path="/runtimes" element={<RuntimesPage />} />
          <Route path="/config" element={<ConfigPage />} />
          <Route path="/logs" element={<LogsPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
