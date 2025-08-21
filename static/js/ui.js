// This module contains all functions that directly manipulate the DOM (UI).

// Import shared state to use it in rendering functions.
import { allServices } from './state.js'

// --- Element Cache ---
// It's good practice to get all DOM elements once.
const elements = {
  mainMenu: document.getElementById('main-menu-view'),
  serviceAccessView: document.getElementById('service-access-view'),
  externalAccessView: document.getElementById('external-access-view'),
  accessRequestHubView: document.getElementById('access-request-hub-view'),
  logDashboard: document.getElementById('results-log-dashboard'),
  logSvc: document.getElementById('results-log-svc'),
  logExt: document.getElementById('results-log-ext'),
  accessListDashboard: document.getElementById('active-access-list-dashboard'),
  accessListSvc: document.getElementById('active-access-list-svc'),
  accessListExt: document.getElementById('active-access-list-ext'),
  pendingRequestsList: document.getElementById('pending-requests-list'),
  noPendingRequestsMessage: document.getElementById(
    'no-pending-requests-message',
  ),
  sourceNsFilter: document.getElementById('ca-source-ns-filter'),
  targetNsFilter: document.getElementById('ca-target-ns-filter'),
  eaNsFilter: document.getElementById('ea-ns-filter'),
  sourceSvcSelect: document.getElementById('ca-source-svc'),
  targetSvcSelect: document.getElementById('ca-target-svc'),
  eaServiceSelect: document.getElementById('ea-service'),
}

export function showView(viewId) {
  document.querySelectorAll('.view').forEach((view) => {
    view.style.display = 'none'
  })
  const viewToShow = document.getElementById(viewId)
  if (viewToShow) {
    viewToShow.style.display = 'block'
  }
}

export function renderLogEntry(entry) {
  const newLog = document.createElement('div')
  newLog.textContent = entry.payload
  newLog.className = entry.className || 'log-info'

  elements.logDashboard.appendChild(newLog.cloneNode(true))
  elements.logDashboard.scrollTop = elements.logDashboard.scrollHeight

  if (entry.logType === 'Service') {
    elements.logSvc.appendChild(newLog.cloneNode(true))
    elements.logSvc.scrollTop = elements.logSvc.scrollHeight
  } else if (entry.logType === 'External') {
    elements.logExt.appendChild(newLog.cloneNode(true))
    elements.logExt.scrollTop = elements.logExt.scrollHeight
  }
}

export function clearLogs() {
  elements.logDashboard.innerHTML = ''
  elements.logSvc.innerHTML = ''
  elements.logExt.innerHTML = ''
}

export function renderAccessList(container, accessData) {
  container.innerHTML = ''
  if (!accessData || accessData.length === 0) {
    container.innerHTML = '<p>No active access policies of this type found.</p>'
    return
  }
  const table = document.createElement('table')
  table.innerHTML = `<thead><tr><th style="width: 60%;">Access Details</th><th>Expires</th><th>Action</th></tr></thead><tbody></tbody>`
  const tbody = table.querySelector('tbody')
  accessData.forEach((access) => {
    const expires =
      access.expiresAt === -1
        ? 'Infinite'
        : new Date(access.expiresAt * 1000).toLocaleTimeString()

    let directionArrow = ''
    let detailsHtml = ''
    const portsDisplay = access.ports || 'Default'
    const rowClass = access.status === 'Pending' ? 'access-pending' : ''

    if (access.type === 'Service') {
      switch (access.direction) {
        case 'ingress':
          directionArrow = '←'
          break
        case 'egress':
          directionArrow = '→'
          break
        case 'all':
        case 'both':
          directionArrow = '↔'
          break
        default:
          directionArrow = '?'
      }
      detailsHtml = `
                    <div style="font-weight: 500; word-break: break-all; display: flex; align-items: center; gap: 8px;">
                        <span>${access.source}</span>
                        <span style="color: var(--md-sys-color-primary); font-size: 1.2em;">${directionArrow}</span>
                        <span>${access.target}</span>
                    </div>
                    <div style="color: #5f6368; font-size: 0.8rem; margin-top: 4px;">Ports: ${portsDisplay}</div>`
    } else {
      switch (access.direction) {
        case 'ingress':
          directionArrow = '→'
          break
        case 'egress':
          directionArrow = '←'
          break
        case 'all':
        case 'both':
          directionArrow = '↔'
          break
        default:
          directionArrow = '?'
      }
      detailsHtml = `
                    <div style="font-weight: 500; word-break: break-all; display: flex; align-items: center; gap: 8px;">
                        <span class="material-symbols-outlined" style="font-size: 1.2em;">public</span>
                        <span>${access.source}</span>
                        <span style="color: var(--md-sys-color-primary); font-size: 1.2em;">${directionArrow}</span>
                        <span>${access.target}</span>
                    </div>
                    <div style="color: #5f6368; font-size: 0.8rem; margin-top: 4px;">Ports: ${portsDisplay}</div>`
    }

    let actionButtonHtml = ''
    if (access.status === 'Pending') {
      actionButtonHtml = `<button class="btn btn-filled btn-small view-request-btn">View Request</button>`
    } else {
      actionButtonHtml = `<button class="btn btn-filled btn-small revoke-btn" data-type="${access.type}" data-name="${access.name}" data-namespace="${access.namespace}">Revoke</button>`
    }

    const row = document.createElement('tr')
    if (rowClass) {
      row.classList.add(rowClass)
    }
    row.innerHTML = `
                <td>${detailsHtml}</td>
                <td>${expires}</td>
                <td>${actionButtonHtml}</td>`
    tbody.appendChild(row)
  })
  container.appendChild(table)
}

export function renderPendingRequests(requests) {
  if (requests && requests.length > 0) {
    elements.pendingRequestsList.style.display = 'block'
    elements.noPendingRequestsMessage.style.display = 'none'
    elements.pendingRequestsList.innerHTML = ''

    const table = document.createElement('table')
    table.innerHTML = `<thead><tr><th>Requestor</th><th>Type</th><th style="width: 35%;">Details</th><th>Duration</th><th>Actions</th></tr></thead><tbody></tbody>`
    const tbody = table.querySelector('tbody')

    requests.forEach((req) => {
      let details = ''
      if (req.requestType === 'Service') {
        details = `<strong>Source:</strong> ${req.sourceService}<br><strong>Target:</strong> ${req.targetService}`
      } else {
        details = `<strong>Source:</strong> ${req.cidr}<br><strong>Target:</strong> ${req.service}`
      }

      let directionText = req.direction
      switch (req.direction) {
        case 'ingress':
          directionText = 'Ingress'
          break
        case 'egress':
          directionText = 'Egress'
          break
        case 'all':
        case 'both':
          directionText = 'All (Ingress & Egress)'
          break
      }
      details += `<br><strong>Direction:</strong> ${directionText}`

      if (req.description) {
        details += `<br><strong>Desc:</strong> ${req.description}`
      }

      let permissionsHtml = ''
      const permissionsList = []

      const getPermsForNs = (ns, resource) => [
        { text: `CREATE services in '${ns}'`, satisfied: false },
        {
          text: `CREATE ${resource}.maxtac.vtk.io in '${ns}'`,
          satisfied: false,
        },
      ]

      if (req.requestType === 'Service') {
        const sourceParts = req.sourceService.split('/')
        const targetParts = req.targetService.split('/')
        if (sourceParts.length === 2 && targetParts.length === 2) {
          const sourcePerms = getPermsForNs(sourceParts[0], 'accesses')
          const targetPerms = getPermsForNs(targetParts[0], 'accesses')

          if (req.status === 'PendingTarget') {
            sourcePerms.forEach((p) => (p.satisfied = true))
          } else if (req.status === 'PendingSource') {
            targetPerms.forEach((p) => (p.satisfied = true))
          }
          permissionsList.push(...sourcePerms, ...targetPerms)
        }
      } else {
        const serviceParts = req.service.split('/')
        if (serviceParts.length === 2) {
          permissionsList.push(
            ...getPermsForNs(serviceParts[0], 'externalaccesses'),
          )
        }
      }

      if (permissionsList.length > 0) {
        const permsAsHtml = permissionsList
          .map(
            (p) =>
              `<small class="${p.satisfied ? 'perm-satisfied' : 'perm-needed'}">${p.text}</small>`,
          )
          .join('<br>')

        permissionsHtml = `<div style="margin-top: 8px; padding-top: 4px; border-top: 1px solid #eee;">
                                    <strong>Permissions Needed:</strong><br>
                                    ${permsAsHtml}
                                    </div>`
      }
      details += permissionsHtml

      let typeAndStatus = req.requestType
      if (req.status && req.status !== 'PendingFull') {
        const friendlyStatus = req.status.replace('Pending', 'Pending ')
        typeAndStatus += `<br><small style="color: var(--log-color-warning);">${friendlyStatus}</small>`
      }

      let actionButtonsHtml = ''
      const isOwner =
        typeof currentUserEmail !== 'undefined' &&
        currentUserEmail === req.requestor

      if (isOwner && req.canSelfApprove) {
        actionButtonsHtml = `
                <div style="display: flex; flex-direction: column; gap: 8px;">
                    <button class="btn btn-filled btn-small approve-btn" data-id="${req.requestID}" style="--md-filled-button-container-height: 32px;">Approve</button>
                    <button class="btn btn-filled btn-small deny-btn" data-id="${req.requestID}" style="--md-filled-button-container-height: 32px;">Abort</button>
                </div>
                `
      } else if (isOwner) {
        actionButtonsHtml = `
                <button class="btn btn-filled btn-small deny-btn" data-id="${req.requestID}" style="--md-filled-button-container-height: 32px;">
                    Abort
                </button>
                `
      } else {
        actionButtonsHtml = `
                <div style="display: flex; flex-direction: column; gap: 8px;">
                    <button class="btn btn-filled btn-small approve-btn" data-id="${req.requestID}" style="--md-filled-button-container-height: 32px;">Approve</button>
                    <button class="btn btn-filled btn-small deny-btn" data-id="${req.requestID}" style="--md-filled-button-container-height: 32px;">Deny</button>
                </div>
                `
      }

      const row = document.createElement('tr')
      row.innerHTML = `
                    <td>${req.requestor}</td>
                    <td>${typeAndStatus}</td>
                    <td>${details}<br><small><strong>Ports:</strong> ${req.ports || 'Default'}</small></td>
                    <td>${req.duration > 0 ? `${req.duration / 60} mins` : 'Infinite'}</td>
                    <td>${actionButtonsHtml}</td>
                `
      tbody.appendChild(row)
    })
    elements.pendingRequestsList.appendChild(table)
  } else {
    elements.pendingRequestsList.style.display = 'none'
    elements.noPendingRequestsMessage.style.display = 'flex'
  }
}

export function updateNamespaceFilters(namespaces) {
  ;[
    elements.sourceNsFilter,
    elements.targetNsFilter,
    elements.eaNsFilter,
  ].forEach((filter) => {
    filter.innerHTML = ''
    const allNsOption = document.createElement('option')
    allNsOption.value = 'all'
    allNsOption.textContent = 'All Namespaces'
    filter.appendChild(allNsOption)
    namespaces.forEach((ns) => {
      const option = document.createElement('option')
      option.value = ns
      option.textContent = ns
      filter.appendChild(option)
    })
  })
}

export function updateServiceDropdown(serviceDropdown, namespaceFilter) {
  serviceDropdown.innerHTML = ''
  const placeholder = document.createElement('option')
  placeholder.value = ''
  placeholder.textContent = 'Select a service...'
  placeholder.disabled = true
  placeholder.selected = true
  serviceDropdown.appendChild(placeholder)

  const filteredServices = allServices.filter(
    (svc) => namespaceFilter === 'all' || svc.namespace === namespaceFilter,
  )

  if (filteredServices.length > 0) {
    filteredServices.forEach((svc) => {
      const option = document.createElement('option')
      option.value = svc.compound
      option.textContent = svc.compound
      serviceDropdown.appendChild(option)
    })
  }
}
