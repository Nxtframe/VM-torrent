/* globals app */

app.controller("DownloadsController", function ($scope, $rootScope, apiget) {

  $scope.$isLoadingFiles = false;
  $scope.$DownloadedFiles = [];
  apiget.files().then(function (xhr) {
    if (xhr.data.Children) {
      $scope.$DownloadedFiles = xhr.data.Children;
    }
  });

  $scope.$expanded = false;
  $scope.section_expanded_toggle = function () {
    $scope.$expanded = !$scope.$expanded;
    if ($scope.$expanded) {
      $scope.$isLoadingFiles = true;
      apiget.files().then(function (xhr) {
        if (xhr.data.Children) {
          $scope.$DownloadedFiles = xhr.data.Children;
        } else {
          $scope.$DownloadedFiles = [];
        }
      }).finally(function () {
        $scope.$isLoadingFiles = false;
        $scope.$applyAsync();
      });
    }
  };
});
