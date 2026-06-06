import React from 'react'
import Sidebar from './Sidebar'

interface Props {
  children: React.ReactNode
}

function SettingsLayout({ children }: Props) {
  return (
    <div style={{ display: 'flex', width: '100%', height: '100vh', background: 'var(--bg-page)' }}>
      <Sidebar />
      <main style={{
        flex: 1,
        overflowY: 'auto',
        padding: 32,
        background: 'var(--bg-page)',
      }}>
        {children}
      </main>
    </div>
  )
}

export default SettingsLayout
