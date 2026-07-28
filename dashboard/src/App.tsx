import { BrowserRouter, Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import DashboardPage from './pages/Dashboard'
import SitesPage from './pages/Sites'
import DeploymentsPage from './pages/Deployments'
import RoutesPage from './pages/Routes'
import RuntimesPage from './pages/Runtimes'
import ConfigPage from './pages/Config'
import LogsPage from './pages/Logs'
import DoctorPage from './pages/Doctor'
import RequestExplorerPage from './pages/RequestExplorer'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<DashboardPage />} />
          <Route path="/sites" element={<SitesPage />} />
          <Route path="/deployments" element={<DeploymentsPage />} />
          <Route path="/routes" element={<RoutesPage />} />
          <Route path="/runtimes" element={<RuntimesPage />} />
          <Route path="/config" element={<ConfigPage />} />
          <Route path="/logs" element={<LogsPage />} />
          <Route path="/doctor" element={<DoctorPage />} />
          <Route path="/explorer" element={<RequestExplorerPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
