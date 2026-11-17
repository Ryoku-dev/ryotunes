import QtQuick

// a fresh array resets a ListView to the top, so an appended page keeps the old contentY
ListView {
    id: view

    property var rows: []
    property var rowKey: (row) => row
    property var applied: []

    function sync() {
        var next = view.rows || [];
        var prev = view.applied;
        var grew = prev.length > 0 && next.length > prev.length;
        for (var i = 0; grew && i < prev.length; i++)
            grew = view.rowKey(prev[i]) === view.rowKey(next[i]);
        var y = view.contentY;
        view.applied = next;
        view.model = next;
        if (grew)
            view.contentY = y;
    }

    onRowsChanged: view.sync()
}
