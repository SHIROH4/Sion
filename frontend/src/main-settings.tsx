import React from 'react'
import {createRoot} from 'react-dom/client'
import './style.css'
import SettingsApp from './SettingsApp'

// Error boundary to catch and display runtime errors.
class ErrorBoundary extends React.Component<
  { children: React.ReactNode },
  { error: Error | null }
> {
  constructor(props: { children: React.ReactNode }) {
    super(props)
    this.state = { error: null }
  }
  static getDerivedStateFromError(error: Error) {
    return { error }
  }
  render() {
    if (this.state.error) {
      return (
        <div style={{
          padding: 40, fontFamily: 'monospace', fontSize: 13,
          color: '#ff4d4f', whiteSpace: 'pre-wrap', wordBreak: 'break-all',
        }}>
          <h3>React Error</h3>
          <p>{this.state.error.message}</p>
          <p style={{ color: '#999', fontSize: 11 }}>{this.state.error.stack}</p>
        </div>
      )
    }
    return this.props.children
  }
}

const container = document.getElementById('root')
if (!container) {
  document.body.innerHTML = '<div style="padding:40px;font-family:monospace;color:red;"><h3>Fatal: #root not found</h3></div>'
} else {
  const root = createRoot(container)
  root.render(
    <React.StrictMode>
      <ErrorBoundary>
        <SettingsApp />
      </ErrorBoundary>
    </React.StrictMode>
  )
}
