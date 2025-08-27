// This is the main entry point for the frontend application.

import { setAllServices, allServices } from './state.js'
import {
  showView,
  renderLogEntry,
  clearLogs,
  renderAccessList,
  renderPendingRequests,
  updateNamespaceFilters,
  updateServiceDropdown,
} from './ui.js'
import { setupEventListeners } from './events.js'

// Re-export functions to make them available to event.js
export {
  initializeCaFilters,
  initializeEaFilters,
  fetchAndDisplayActiveAccesses,
  fetchAndRenderPendingRequests,
}

// --- DATA FETCHING AND STATE MANAGEMENT ---
async function fetchAllServices() {
  try {
    const response = await fetch('/api/services')
    if (!response.ok) throw new Error('Failed to fetch services')
    const services = await response.json()
    setAllServices(services)
  } catch (error) {
    console.error('Error fetching master service list:', error)
    renderLogEntry({
      payload: 'Could not load services from the cluster.',
      className: 'log-error',
      logType: 'Global',
    })
  }
}

async function fetchAndRenderLogs() {
  try {
    const response = await fetch('/api/logs')
    if (!response.ok) throw new Error('Failed to fetch logs')
    const logs = await response.json()
    clearLogs()
    logs.forEach(renderLogEntry)
  } catch (error) {
    console.error('Error fetching activity log:', error)
    renderLogEntry({
      payload: 'Could not load historical activity log.',
      className: 'log-error',
      logType: 'Global',
    })
  }
}

async function fetchAndDisplayActiveAccesses() {
  document.getElementById('refresh-access-btn-dashboard').disabled = true
  document.getElementById('refresh-access-btn-svc').disabled = true
  document.getElementById('refresh-access-btn-ext').disabled = true
  document.getElementById('active-access-list-dashboard').innerHTML =
    '<p>Refreshing...</p>'
  document.getElementById('active-access-list-svc').innerHTML =
    '<p>Refreshing...</p>'
  document.getElementById('active-access-list-ext').innerHTML =
    '<p>Refreshing...</p>'

  try {
    const response = await fetch('/api/active-accesses')
    if (!response.ok) throw new Error('Failed to fetch active accesses')
    const allActiveAccesses = await response.json()

    renderAccessList(
      document.getElementById('active-access-list-dashboard'),
      allActiveAccesses,
    )
    renderAccessList(
      document.getElementById('active-access-list-svc'),
      allActiveAccesses.filter((a) => a.type === 'Service'),
    )
    renderAccessList(
      document.getElementById('active-access-list-ext'),
      allActiveAccesses.filter((a) => a.type === 'External'),
    )
  } catch (error) {
    console.error('Error fetching active accesses:', error)
    ;[
      document.getElementById('active-access-list-dashboard'),
      document.getElementById('active-access-list-svc'),
      document.getElementById('active-access-list-ext'),
    ].forEach(
      (el) =>
        (el.innerHTML =
          '<p class="log-error">Could not load active accesses.</p>'),
    )
  } finally {
    document.getElementById('refresh-access-btn-dashboard').disabled = false
    document.getElementById('refresh-access-btn-svc').disabled = false
    document.getElementById('refresh-access-btn-ext').disabled = false
  }
}

async function fetchAndRenderPendingRequests() {
  document.getElementById('pending-requests-list').innerHTML =
    '<p>Refreshing pending requests...</p>'
  try {
    const response = await fetch('/api/pending-requests')
    if (!response.ok) throw new Error('Failed to fetch pending requests')
    const requests = await response.json()
    renderPendingRequests(requests)
  } catch (error) {
    console.error('Error fetching pending requests:', error)
    document.getElementById('pending-requests-list').innerHTML =
      '<p class="log-error">Could not load pending requests.</p>'
  }
}

// --- INITIALIZATION ---
let serviceViewInitialized = false
let externalViewInitialized = false

async function initializeCaFilters() {
  if (serviceViewInitialized) {
    await fetchAllServices()
  }
  if (!serviceViewInitialized) {
    await fetchAllServices()
    serviceViewInitialized = true
  }
  const namespaces = [
    ...new Set(allServices.map((svc) => svc.namespace)),
  ].sort()
  updateNamespaceFilters(namespaces)
  updateServiceDropdown(document.getElementById('ca-source-svc'), 'all')
  updateServiceDropdown(document.getElementById('ca-target-svc'), 'all')
}

async function initializeEaFilters() {
  if (externalViewInitialized) {
    await fetchAllServices()
  }
  if (!externalViewInitialized) {
    await fetchAllServices()
    externalViewInitialized = true
  }
  const namespaces = [
    ...new Set(allServices.map((svc) => svc.namespace)),
  ].sort()
  updateNamespaceFilters(namespaces)
  updateServiceDropdown(document.getElementById('ea-service'), 'all')
}

function selectivelyResetCaForm() {
  document.getElementById('ca-ports').value = ''
  document.getElementById('ca-description').value = ''
}

function selectivelyResetEaForm() {
  document.getElementById('ea-cidr').value = ''
  document.getElementById('ea-ports').value = ''
  document.getElementById('ea-description').value = ''
}

// --- MAIN EXECUTION & WEBSOCKET MANAGEMENT ---
document.addEventListener('DOMContentLoaded', () => {
  let socket
  let pingTimeout

  function connect() {
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    socket = new WebSocket(`${wsProtocol}//${window.location.host}/ws`)

    const heartbeat = () => {
      clearTimeout(pingTimeout)
      // The server sends a ping every ~54 seconds. If we don't get a pong back
      // (or any other message) within 60s + a grace period, we assume the
      // connection is dead and the onclose handler will trigger a reconnect.
      pingTimeout = setTimeout(() => {
        console.log('WebSocket timeout, terminating and reconnecting...')
        socket.close()
      }, 60000 + 1000) // 60s pongWait + 1s grace period.
    }

    socket.onopen = () => {
      console.log('WebSocket connection established.')
      renderLogEntry({
        payload: 'Connection established.',
        className: 'log-success',
        logType: 'Global',
      })
      heartbeat()
      fetchAndRenderLogs()
      fetchAndDisplayActiveAccesses()
    }

    socket.onclose = (event) => {
      console.log('WebSocket connection closed.', event)
      clearTimeout(pingTimeout)
      renderLogEntry({
        payload: 'Connection lost. Attempting to reconnect in 5 seconds...',
        className: 'log-warning',
        logType: 'Global',
      })
      setTimeout(() => {
        connect()
      }, 5000)
    }

    socket.onmessage = (event) => {
      heartbeat() // Reset the timeout on any incoming message
      const data = JSON.parse(event.data)
      renderLogEntry(data)

      const isApprovalOrDenial =
        data.payload.includes('approved') ||
        data.payload.includes('denied') ||
        data.payload.includes('aborted')
      const isCreationComplete = data.type === 'applyComplete'
      const isRevocationInitiated = data.payload.includes(
        'Revocation initiated',
      )
      const isSuccessfulSubmission = data.payload.includes(
        'submitted for review',
      )

      if (isCreationComplete || isApprovalOrDenial || isSuccessfulSubmission) {
        fetchAndDisplayActiveAccesses()
        if (
          document.getElementById('access-request-hub-view').style.display ===
          'block'
        ) {
          fetchAndRenderPendingRequests()
        }
      }

      if (isRevocationInitiated) {
        setTimeout(() => {
          fetchAndDisplayActiveAccesses()
        }, 3000)
      }

      if (isSuccessfulSubmission) {
        if (
          document.getElementById('service-access-view').style.display ===
            'block' &&
          (data.logType === 'Service' || data.logType === 'Request')
        ) {
          selectivelyResetCaForm()
        }
        if (
          document.getElementById('external-access-view').style.display ===
            'block' &&
          (data.logType === 'External' || data.logType === 'Request')
        ) {
          selectivelyResetEaForm()
        }
      }
    }

    socket.onerror = (error) => {
      console.error('WebSocket error:', error)
      // The browser will automatically trigger the 'onclose' event after an error,
      // so we don't need to call connect() here.
      socket.close()
    }

    // Pass the socket instance to the event listeners module
    setupEventListeners(socket, {
      initializeCaFilters,
      initializeEaFilters,
      fetchAndDisplayActiveAccesses,
      fetchAndRenderPendingRequests,
    })
  }

  // Initial connection
  connect()

  showView('main-menu-view')
})
